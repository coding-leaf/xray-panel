package xray

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"panel/internal/pkg/jsonc"

	"go.etcd.io/bbolt"
)

// SyncToDiskConfig synchronizes all current inbound users stored in BoltDB back into
// a disk config.json file (based on templatePath) and atomically saves to targetPath.
// This is used upon server shutdown or manual administrative trigger to ensure zero cold-boot loss.
func (c *XrayClient) SyncToDiskConfig(templatePath, targetPath string) error {
	if c.db == nil {
		return ErrStorageNotAvailable
	}
	return SyncToDiskConfig(c.db, templatePath, targetPath)
}

// SyncToDiskConfig reads user records from db and injects them into the template config,
// writing the resulting JSON atomically to targetPath.
func SyncToDiskConfig(db *bbolt.DB, templatePath, targetPath string) error {
	if db == nil {
		return ErrStorageNotAvailable
	}
	templatePath = strings.TrimSpace(templatePath)
	targetPath = strings.TrimSpace(targetPath)

	if templatePath == "" {
		return fmt.Errorf("%w: templatePath cannot be empty", ErrInvalidParameter)
	}
	if targetPath == "" {
		return fmt.Errorf("%w: targetPath cannot be empty", ErrInvalidParameter)
	}

	// 1. Fetch all managed inbound tags and user records atomically from storage in a single transaction
	managedTags := make(map[string]bool)
	usersByTag := make(map[string][]*InboundUser)

	err := db.View(func(tx *bbolt.Tx) error {
		if mb := tx.Bucket([]byte(BucketManagedInbounds)); mb != nil {
			c := mb.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				managedTags[string(k)] = true
			}
		}
		if b := tx.Bucket([]byte(BucketInboundUsers)); b != nil {
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var u InboundUser
				if unmarshalErr := json.Unmarshal(v, &u); unmarshalErr == nil {
					usersByTag[u.InboundTag] = append(usersByTag[u.InboundTag], &u)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to fetch users from storage: %w", err)
	}

	// 2. Read base/template config file
	rawBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template config from %s: %w", templatePath, err)
	}

	// Strip comments (JSONC)
	cleaned := jsonc.StripJSONC(rawBytes)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return fmt.Errorf("template config is not valid JSON: %w", err)
	}

	// 3. Inspect inbounds array
	inboundsRaw, ok := root["inbounds"]
	if !ok {
		return fmt.Errorf("template config missing 'inbounds' array")
	}
	inboundsList, ok := inboundsRaw.([]interface{})
	if !ok {
		return fmt.Errorf("template config 'inbounds' field is not an array")
	}

	// 4. Update managed inbounds with latest users
	for i, inbRaw := range inboundsList {
		inbMap, ok := inbRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := inbMap["tag"].(string)
		if tag == "" {
			continue
		}

		dbUsers, hasUsers := usersByTag[tag]
		isManaged := managedTags[tag]

		// Only rewrite inbounds that are managed in DB or have users registered
		if !isManaged && !hasUsers {
			continue
		}

		protocol, _ := inbMap["protocol"].(string)
		protocolLower := strings.ToLower(protocol)

		var settings map[string]interface{}
		if s, ok := inbMap["settings"].(map[string]interface{}); ok && s != nil {
			settings = s
		} else {
			settings = make(map[string]interface{})
		}

		clients := make([]map[string]interface{}, 0, len(dbUsers))
		for _, u := range dbUsers {
			client := map[string]interface{}{
				"email": u.Email,
				"level": 0,
			}

			switch protocolLower {
			case "vless":
				client["id"] = u.UUID
				var streamSettings map[string]interface{}
				if ss, ok := inbMap["streamSettings"].(map[string]interface{}); ok && ss != nil {
					streamSettings = ss
				}
				net, _ := streamSettings["network"].(string)
				netLower := strings.ToLower(net)
				isNonTCP := netLower != "" && netLower != "tcp"
				if !isNonTCP && u.Flow != "" && u.Flow != "none" {
					client["flow"] = u.Flow
				}
			case "vmess":
				client["id"] = u.UUID
			case "trojan":
				client["password"] = u.UUID
			case "shadowsocks":
				client["password"] = u.UUID
				method := u.Flow
				if method == "" {
					if m, ok := settings["method"].(string); ok && m != "" {
						method = m
					} else {
						method = "aes-128-gcm"
					}
				}
				client["method"] = method
			default:
				client["id"] = u.UUID
				if u.Flow != "" {
					client["flow"] = u.Flow
				}
			}
			clients = append(clients, client)
		}

		// Ensure mandatory Xray protocol constraints are satisfied
		if protocolLower == "vless" {
			if settings["decryption"] == nil || settings["decryption"] == "" {
				settings["decryption"] = "none"
			}
		}

		settings["clients"] = clients
		inbMap["settings"] = settings
		inboundsList[i] = inbMap
	}
	root["inbounds"] = inboundsList

	// 5. Serialize formatted JSON
	updatedJSON, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	// 6. Atomically write to targetPath using a temp file in the target directory
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	tmpFile, err := os.CreateTemp(targetDir, "xray-config-sync-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(updatedJSON); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write updated config to temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Preserve original file permissions if possible
	fileMode := os.FileMode(0644)
	if fi, err := os.Stat(templatePath); err == nil {
		fileMode = fi.Mode()
	} else if fi, err := os.Stat(targetPath); err == nil {
		fileMode = fi.Mode()
	}
	_ = os.Chmod(tmpPath, fileMode)

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to atomically rename %s to %s: %w", tmpPath, targetPath, err)
	}

	success = true
	return nil
}
