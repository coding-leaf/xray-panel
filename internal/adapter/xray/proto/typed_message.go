package proto

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

const (
	TypeVLESSAccount        = "xray.proxy.vless.Account"
	TypeVMessAccount        = "xray.proxy.vmess.Account"
	TypeTrojanAccount       = "xray.proxy.trojan.Account"
	TypeShadowsocksAccount  = "xray.proxy.shadowsocks.Account"
	TypeAddUserOperation    = "xray.app.proxyman.command.AddUserOperation"
	TypeRemoveUserOperation = "xray.app.proxyman.command.RemoveUserOperation"
)

type VlessAccount = VLESSAccount
type VmessAccount = VMessAccount

// ToTypedMessage converts a proto Message into TypedMessage with Xray compatible type names.
func ToTypedMessage(msg proto.Message) (*TypedMessage, error) {
	if msg == nil {
		return nil, nil
	}
	var typeName string
	switch msg.(type) {
	case *VLESSAccount:
		typeName = TypeVLESSAccount
	case *VMessAccount:
		typeName = TypeVMessAccount
	case *TrojanAccount:
		typeName = TypeTrojanAccount
	case *ShadowsocksAccount:
		typeName = TypeShadowsocksAccount
	case *AddUserOperation:
		typeName = TypeAddUserOperation
	case *RemoveUserOperation:
		typeName = TypeRemoveUserOperation
	default:
		typeName = string(msg.ProtoReflect().Descriptor().FullName())
	}

	b, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal %s failed: %w", typeName, err)
	}
	return &TypedMessage{
		Type:  typeName,
		Value: b,
	}, nil
}

// MustToTypedMessage converts a proto Message into TypedMessage and panics on error.
func MustToTypedMessage(msg proto.Message) *TypedMessage {
	tm, err := ToTypedMessage(msg)
	if err != nil {
		panic(err)
	}
	return tm
}

// GetInstance converts current TypedMessage into a proto.Message instance.
func (m *TypedMessage) GetInstance() (proto.Message, error) {
	if m == nil {
		return nil, nil
	}
	var target proto.Message
	switch m.Type {
	case TypeVLESSAccount:
		target = new(VLESSAccount)
	case TypeVMessAccount:
		target = new(VMessAccount)
	case TypeTrojanAccount:
		target = new(TrojanAccount)
	case TypeShadowsocksAccount:
		target = new(ShadowsocksAccount)
	case TypeAddUserOperation:
		target = new(AddUserOperation)
	case TypeRemoveUserOperation:
		target = new(RemoveUserOperation)
	default:
		return nil, fmt.Errorf("unsupported typed message type: %s", m.Type)
	}

	if err := proto.Unmarshal(m.Value, target); err != nil {
		return nil, fmt.Errorf("unmarshal %s failed: %w", m.Type, err)
	}
	return target, nil
}
