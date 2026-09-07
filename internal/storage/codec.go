package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// SerializerType specifies the serialization format for values.
type SerializerType int

const (
	// SerializerBinary uses a fixed 25-byte big-endian compact binary format.
	SerializerBinary SerializerType = iota
	// SerializerJSON uses standard compact JSON encoding.
	SerializerJSON
)

const (
	binaryFormatVersion byte = 0x01
	keyDelimiter        byte = 0x00
)

// makeHistoryKey generates a composite key: <email>\x00<big_endian_uint64_timestamp>
func makeHistoryKey(email string, timestamp int64) []byte {
	if timestamp < 0 {
		timestamp = 0
	}
	key := make([]byte, len(email)+1+8)
	copy(key, email)
	key[len(email)] = keyDelimiter
	binary.BigEndian.PutUint64(key[len(email)+1:], uint64(timestamp))
	return key
}

// makeHistoryPrefix generates the prefix for seeking records of a given email: <email>\x00
func makeHistoryPrefix(email string) []byte {
	prefix := make([]byte, len(email)+1)
	copy(prefix, email)
	prefix[len(email)] = keyDelimiter
	return prefix
}

// extractTimestampFromKey extracts the int64 timestamp from a history composite key.
func extractTimestampFromKey(key []byte, prefixLen int) int64 {
	if len(key) < prefixLen+8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(key[prefixLen : prefixLen+8]))
}

// encodeUserTraffic serializes UserTraffic based on SerializerType.
func encodeUserTraffic(u *UserTraffic, st SerializerType) ([]byte, error) {
	if st == SerializerJSON {
		return json.Marshal(u)
	}
	// Binary layout (25 bytes):
	// [0]: version (0x01)
	// [1:9]: Uplink (int64 BigEndian)
	// [9:17]: Downlink (int64 BigEndian)
	// [17:25]: LastSeen (int64 BigEndian)
	buf := make([]byte, 25)
	buf[0] = binaryFormatVersion
	binary.BigEndian.PutUint64(buf[1:9], uint64(u.Uplink))
	binary.BigEndian.PutUint64(buf[9:17], uint64(u.Downlink))
	binary.BigEndian.PutUint64(buf[17:25], uint64(u.LastSeen))
	return buf, nil
}

// decodeUserTraffic decodes UserTraffic, auto-detecting binary vs JSON.
func decodeUserTraffic(data []byte) (*UserTraffic, error) {
	if len(data) == 0 {
		return nil, errors.New("cannot decode empty user traffic data")
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var u UserTraffic
		if err := json.Unmarshal(trimmed, &u); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON user traffic: %w", err)
		}
		return &u, nil
	}

	if data[0] == binaryFormatVersion {
		if len(data) < 25 {
			return nil, fmt.Errorf("binary data too short for UserTraffic: expected 25 bytes, got %d", len(data))
		}
		return &UserTraffic{
			Uplink:   int64(binary.BigEndian.Uint64(data[1:9])),
			Downlink: int64(binary.BigEndian.Uint64(data[9:17])),
			LastSeen: int64(binary.BigEndian.Uint64(data[17:25])),
		}, nil
	}

	return nil, fmt.Errorf("unsupported user traffic encoding format: 0x%02x", data[0])
}

// encodeTrafficSnapshot serializes TrafficSnapshot based on SerializerType.
func encodeTrafficSnapshot(s *TrafficSnapshot, st SerializerType) ([]byte, error) {
	if st == SerializerJSON {
		return json.Marshal(s)
	}
	// Binary layout (25 bytes):
	// [0]: version (0x01)
	// [1:9]: Uplink (int64 BigEndian)
	// [9:17]: Downlink (int64 BigEndian)
	// [17:25]: Timestamp (int64 BigEndian)
	buf := make([]byte, 25)
	buf[0] = binaryFormatVersion
	binary.BigEndian.PutUint64(buf[1:9], uint64(s.Uplink))
	binary.BigEndian.PutUint64(buf[9:17], uint64(s.Downlink))
	binary.BigEndian.PutUint64(buf[17:25], uint64(s.Timestamp))
	return buf, nil
}

// decodeTrafficSnapshot decodes TrafficSnapshot, auto-detecting binary vs JSON.
func decodeTrafficSnapshot(data []byte) (*TrafficSnapshot, error) {
	if len(data) == 0 {
		return nil, errors.New("cannot decode empty traffic snapshot data")
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var s TrafficSnapshot
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON traffic snapshot: %w", err)
		}
		return &s, nil
	}

	if data[0] == binaryFormatVersion {
		if len(data) < 25 {
			return nil, fmt.Errorf("binary data too short for TrafficSnapshot: expected 25 bytes, got %d", len(data))
		}
		return &TrafficSnapshot{
			Uplink:    int64(binary.BigEndian.Uint64(data[1:9])),
			Downlink:  int64(binary.BigEndian.Uint64(data[9:17])),
			Timestamp: int64(binary.BigEndian.Uint64(data[17:25])),
		}, nil
	}

	return nil, fmt.Errorf("unsupported traffic snapshot encoding format: 0x%02x", data[0])
}
