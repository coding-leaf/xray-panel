package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	proxymanCommand "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
)

// CommandAddUserRequest encapsulates all parameters required to construct an AddUser command
// for Xray's HandlerService AlterInbound RPC.
type CommandAddUserRequest struct {
	InboundTag string `json:"inbound_tag"`
	Email      string `json:"email"`
	UUID       string `json:"uuid"`
	Flow       string `json:"flow,omitempty"`     // E.g. "xtls-rprx-vision" (VLESS) or cipher name (Shadowsocks)
	Protocol   string `json:"protocol,omitempty"` // "vless", "vmess", "trojan", "shadowsocks". Defaults to "vless"
	Level      uint32 `json:"level,omitempty"`    // User level, defaults to 0
}

// CommandRemoveUserRequest encapsulates parameters required to construct a RemoveUser command
// for Xray's HandlerService AlterInbound RPC.
type CommandRemoveUserRequest struct {
	InboundTag string `json:"inbound_tag"`
	Email      string `json:"email"`
}

// BuildCommandAddUserRequest constructs an AlterInboundRequest containing an AddUserOperation
// with standard VLESS protocol defaults.
func BuildCommandAddUserRequest(inboundTag, email, uuid, flow string) (*proxymanCommand.AlterInboundRequest, error) {
	return BuildCommandAddUserRequestAdvanced(&CommandAddUserRequest{
		InboundTag: inboundTag,
		Email:      email,
		UUID:       uuid,
		Flow:       flow,
		Protocol:   "vless",
	})
}

// BuildCommandAddUserRequestAdvanced constructs an AlterInboundRequest supporting polymorphic protocols
// (VLESS, VMess, Trojan, Shadowsocks).
func BuildCommandAddUserRequestAdvanced(req *CommandAddUserRequest) (*proxymanCommand.AlterInboundRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request cannot be nil", ErrInvalidParameter)
	}
	req.InboundTag = strings.TrimSpace(req.InboundTag)
	req.Email = strings.TrimSpace(req.Email)
	req.UUID = strings.TrimSpace(req.UUID)
	req.Flow = strings.TrimSpace(req.Flow)
	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))

	if req.InboundTag == "" {
		return nil, fmt.Errorf("%w: inboundTag cannot be empty", ErrInvalidParameter)
	}
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email cannot be empty", ErrInvalidParameter)
	}
	if req.UUID == "" {
		return nil, fmt.Errorf("%w: uuid cannot be empty", ErrInvalidParameter)
	}
	if req.Protocol == "" {
		req.Protocol = "vless"
	}

	accountMsg, err := buildAccountTypedMessage(req.Protocol, req.UUID, req.Flow)
	if err != nil {
		return nil, err
	}

	protoUser := &protocol.User{
		Level:   req.Level,
		Email:   req.Email,
		Account: accountMsg,
	}

	return &proxymanCommand.AlterInboundRequest{
		Tag: req.InboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.AddUserOperation{
			User: protoUser,
		}),
	}, nil
}

// BuildCommandRemoveUserRequest constructs an AlterInboundRequest containing a RemoveUserOperation.
func BuildCommandRemoveUserRequest(inboundTag, email string) (*proxymanCommand.AlterInboundRequest, error) {
	inboundTag = strings.TrimSpace(inboundTag)
	email = strings.TrimSpace(email)
	if inboundTag == "" {
		return nil, fmt.Errorf("%w: inboundTag cannot be empty", ErrInvalidParameter)
	}
	if email == "" {
		return nil, fmt.Errorf("%w: email cannot be empty", ErrInvalidParameter)
	}

	return &proxymanCommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.RemoveUserOperation{
			Email: email,
		}),
	}, nil
}

// buildAccountTypedMessage serializes account credentials into the appropriate TypedMessage for Xray polymorphic dispatch.
func buildAccountTypedMessage(protoName, uuid, flow string) (*serial.TypedMessage, error) {
	switch strings.ToLower(strings.TrimSpace(protoName)) {
	case "vless":
		return serial.ToTypedMessage(&vless.Account{
			Id:   uuid,
			Flow: flow,
		}), nil
	case "vmess":
		return serial.ToTypedMessage(&vmess.Account{
			Id: uuid,
			SecuritySettings: &protocol.SecurityConfig{
				Type: protocol.SecurityType_AUTO,
			},
		}), nil
	case "trojan":
		return serial.ToTypedMessage(&trojan.Account{
			Password: uuid,
		}), nil
	case "shadowsocks":
		cipher := flow
		if cipher == "" {
			cipher = "aes-128-gcm"
		}
		var cipherType shadowsocks.CipherType
		switch strings.ToLower(strings.TrimSpace(cipher)) {
		case "aes-256-gcm":
			cipherType = shadowsocks.CipherType_AES_256_GCM
		case "chacha20-poly1305", "chacha20-ietf-poly1305":
			cipherType = shadowsocks.CipherType_CHACHA20_POLY1305
		case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
			cipherType = shadowsocks.CipherType_XCHACHA20_POLY1305
		case "none":
			cipherType = shadowsocks.CipherType_NONE
		case "aes-128-gcm":
			fallthrough
		default:
			cipherType = shadowsocks.CipherType_AES_128_GCM
		}
		return serial.ToTypedMessage(&shadowsocks.Account{
			Password:   uuid,
			CipherType: cipherType,
		}), nil
	default:
		return nil, fmt.Errorf("%w: unsupported protocol %q", ErrInvalidParameter, protoName)
	}
}

// AddInboundUser dynamically pushes a new user credential to the running Xray-core instance via HandlerService.
// If the gRPC call succeeds and local BoltDB storage is configured, the user is persisted to local storage.
// If the gRPC call fails, the database is untouched to avoid phantom/dirty state.
func (c *XrayClient) AddInboundUser(inboundTag string, email string, uuid string, flow string) error {
	return c.AddInboundUserWithContext(context.Background(), inboundTag, email, uuid, flow)
}

// AddInboundUserWithContext executes AddInboundUser with caller-provided context.
func (c *XrayClient) AddInboundUserWithContext(ctx context.Context, inboundTag string, email string, uuid string, flow string) error {
	return c.AddInboundUserAdvanced(ctx, &CommandAddUserRequest{
		InboundTag: inboundTag,
		Email:      email,
		UUID:       uuid,
		Flow:       flow,
		Protocol:   "vless",
	})
}

// AddInboundUserWithProtocol dynamically pushes a user with specified protocol (vless, vmess, trojan, shadowsocks).
func (c *XrayClient) AddInboundUserWithProtocol(inboundTag, email, uuid, flow, protocolName string) error {
	return c.AddInboundUserAdvanced(context.Background(), &CommandAddUserRequest{
		InboundTag: inboundTag,
		Email:      email,
		UUID:       uuid,
		Flow:       flow,
		Protocol:   protocolName,
	})
}

// AddInboundUserAdvanced handles the complete lifecycle of adding a user to runtime and storage,
// including parameter validation, CommandAddUserRequest construction, gRPC dispatch, and compensation rollback.
func (c *XrayClient) AddInboundUserAdvanced(ctx context.Context, req *CommandAddUserRequest) error {
	alterReq, err := BuildCommandAddUserRequestAdvanced(req)
	if err != nil {
		return err
	}

	// 1. Ensure gRPC connection is established
	_, err = c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get gRPC connection: %w", err)
	}

	// 2. Execute gRPC call with auto-retry on transient connection failure
	err = c.withRetry(ctx, func() error {
		c.mu.RLock()
		handler := c.handlerCli
		c.mu.RUnlock()

		if handler == nil {
			return errors.New("handler client not available")
		}
		_, rpcErr := handler.AlterInbound(ctx, alterReq)
		return rpcErr
	})
	if err != nil {
		return fmt.Errorf("xray alter inbound add user failed: %w", err)
	}

	// 3. Persistence coordination: only write to BoltDB after gRPC confirmation
	if c.db != nil {
		proto := req.Protocol
		if proto == "" {
			proto = "vless"
		}
		userRecord := &InboundUser{
			InboundTag: req.InboundTag,
			Email:      req.Email,
			UUID:       req.UUID,
			Flow:       req.Flow,
			Protocol:   proto,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if dbErr := c.saveUserToDB(userRecord); dbErr != nil {
			// Compensation rollback: attempt to remove user from Xray runtime to prevent phantom state
			rollbackReq, rbErr := BuildCommandRemoveUserRequest(req.InboundTag, req.Email)
			if rbErr == nil {
				c.mu.RLock()
				handler := c.handlerCli
				c.mu.RUnlock()
				if handler != nil {
					_, _ = handler.AlterInbound(ctx, rollbackReq)
				}
			}
			return fmt.Errorf("user added to xray runtime but failed to persist to local storage (compensation rollback attempted): %w", dbErr)
		}
	}

	return nil
}

// RemoveInboundUser dynamically removes an existing user from the running Xray-core instance via HandlerService.
// If the gRPC call succeeds and local BoltDB storage is configured, the user is removed from local storage.
// If the gRPC call fails, the database is untouched.
func (c *XrayClient) RemoveInboundUser(inboundTag string, email string) error {
	return c.RemoveInboundUserWithContext(context.Background(), inboundTag, email)
}

// RemoveInboundUserWithContext executes RemoveInboundUser with caller-provided context.
func (c *XrayClient) RemoveInboundUserWithContext(ctx context.Context, inboundTag string, email string) error {
	return c.RemoveInboundUserAdvanced(ctx, &CommandRemoveUserRequest{
		InboundTag: inboundTag,
		Email:      email,
	})
}

// RemoveInboundUserAdvanced removes a user dynamically using a CommandRemoveUserRequest specification.
func (c *XrayClient) RemoveInboundUserAdvanced(ctx context.Context, req *CommandRemoveUserRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request cannot be nil", ErrInvalidParameter)
	}
	alterReq, err := BuildCommandRemoveUserRequest(req.InboundTag, req.Email)
	if err != nil {
		return err
	}

	// 1. Ensure gRPC connection is established
	_, err = c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get gRPC connection: %w", err)
	}

	// 2. Execute gRPC call with auto-retry on transient connection failure
	err = c.withRetry(ctx, func() error {
		c.mu.RLock()
		handler := c.handlerCli
		c.mu.RUnlock()

		if handler == nil {
			return errors.New("handler client not available")
		}
		_, rpcErr := handler.AlterInbound(ctx, alterReq)
		return rpcErr
	})
	if err != nil {
		return fmt.Errorf("xray alter inbound remove user failed: %w", err)
	}

	// 3. Persistence coordination: only remove from BoltDB after gRPC confirmation
	if c.db != nil {
		if dbErr := c.removeUserFromDB(req.InboundTag, req.Email); dbErr != nil {
			return fmt.Errorf("user removed from xray runtime but failed to delete from local storage: %w", dbErr)
		}
	}

	return nil
}
