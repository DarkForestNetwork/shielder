// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0
// 	protoc        v3.13.0
// source: shmsg/shmsg.proto

package shmsg

import (
	proto "github.com/golang/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type PublicKeyCommitment struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BatchIndex uint64 `protobuf:"varint,1,opt,name=batchIndex,proto3" json:"batchIndex,omitempty"`
	Commitment []byte `protobuf:"bytes,2,opt,name=commitment,proto3" json:"commitment,omitempty"`
}

func (x *PublicKeyCommitment) Reset() {
	*x = PublicKeyCommitment{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shmsg_shmsg_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PublicKeyCommitment) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PublicKeyCommitment) ProtoMessage() {}

func (x *PublicKeyCommitment) ProtoReflect() protoreflect.Message {
	mi := &file_shmsg_shmsg_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PublicKeyCommitment.ProtoReflect.Descriptor instead.
func (*PublicKeyCommitment) Descriptor() ([]byte, []int) {
	return file_shmsg_shmsg_proto_rawDescGZIP(), []int{0}
}

func (x *PublicKeyCommitment) GetBatchIndex() uint64 {
	if x != nil {
		return x.BatchIndex
	}
	return 0
}

func (x *PublicKeyCommitment) GetCommitment() []byte {
	if x != nil {
		return x.Commitment
	}
	return nil
}

type PublicKeyShare struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BatchId string `protobuf:"bytes,1,opt,name=batch_id,json=batchId,proto3" json:"batch_id,omitempty"`
	Key     []byte `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
}

func (x *PublicKeyShare) Reset() {
	*x = PublicKeyShare{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shmsg_shmsg_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PublicKeyShare) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PublicKeyShare) ProtoMessage() {}

func (x *PublicKeyShare) ProtoReflect() protoreflect.Message {
	mi := &file_shmsg_shmsg_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PublicKeyShare.ProtoReflect.Descriptor instead.
func (*PublicKeyShare) Descriptor() ([]byte, []int) {
	return file_shmsg_shmsg_proto_rawDescGZIP(), []int{1}
}

func (x *PublicKeyShare) GetBatchId() string {
	if x != nil {
		return x.BatchId
	}
	return ""
}

func (x *PublicKeyShare) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

type SecretShare struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BatchIndex uint64 `protobuf:"varint,1,opt,name=batchIndex,proto3" json:"batchIndex,omitempty"`
	Privkey    []byte `protobuf:"bytes,2,opt,name=privkey,proto3" json:"privkey,omitempty"`
}

func (x *SecretShare) Reset() {
	*x = SecretShare{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shmsg_shmsg_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SecretShare) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SecretShare) ProtoMessage() {}

func (x *SecretShare) ProtoReflect() protoreflect.Message {
	mi := &file_shmsg_shmsg_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SecretShare.ProtoReflect.Descriptor instead.
func (*SecretShare) Descriptor() ([]byte, []int) {
	return file_shmsg_shmsg_proto_rawDescGZIP(), []int{2}
}

func (x *SecretShare) GetBatchIndex() uint64 {
	if x != nil {
		return x.BatchIndex
	}
	return 0
}

func (x *SecretShare) GetPrivkey() []byte {
	if x != nil {
		return x.Privkey
	}
	return nil
}

type BatchConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	StartBatchIndex uint64   `protobuf:"varint,1,opt,name=start_batch_index,json=startBatchIndex,proto3" json:"start_batch_index,omitempty"`
	Keypers         [][]byte `protobuf:"bytes,2,rep,name=keypers,proto3" json:"keypers,omitempty"`
	Threshold       uint32   `protobuf:"varint,3,opt,name=threshold,proto3" json:"threshold,omitempty"`
}

func (x *BatchConfig) Reset() {
	*x = BatchConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shmsg_shmsg_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BatchConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BatchConfig) ProtoMessage() {}

func (x *BatchConfig) ProtoReflect() protoreflect.Message {
	mi := &file_shmsg_shmsg_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BatchConfig.ProtoReflect.Descriptor instead.
func (*BatchConfig) Descriptor() ([]byte, []int) {
	return file_shmsg_shmsg_proto_rawDescGZIP(), []int{3}
}

func (x *BatchConfig) GetStartBatchIndex() uint64 {
	if x != nil {
		return x.StartBatchIndex
	}
	return 0
}

func (x *BatchConfig) GetKeypers() [][]byte {
	if x != nil {
		return x.Keypers
	}
	return nil
}

func (x *BatchConfig) GetThreshold() uint32 {
	if x != nil {
		return x.Threshold
	}
	return 0
}

type Message struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Payload:
	//	*Message_PublicKeyCommitment
	//	*Message_PublicKeyShare
	//	*Message_SecretShare
	//	*Message_BatchConfig
	Payload isMessage_Payload `protobuf_oneof:"payload"`
}

func (x *Message) Reset() {
	*x = Message{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shmsg_shmsg_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Message) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Message) ProtoMessage() {}

func (x *Message) ProtoReflect() protoreflect.Message {
	mi := &file_shmsg_shmsg_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Message.ProtoReflect.Descriptor instead.
func (*Message) Descriptor() ([]byte, []int) {
	return file_shmsg_shmsg_proto_rawDescGZIP(), []int{4}
}

func (m *Message) GetPayload() isMessage_Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (x *Message) GetPublicKeyCommitment() *PublicKeyCommitment {
	if x, ok := x.GetPayload().(*Message_PublicKeyCommitment); ok {
		return x.PublicKeyCommitment
	}
	return nil
}

func (x *Message) GetPublicKeyShare() *PublicKeyShare {
	if x, ok := x.GetPayload().(*Message_PublicKeyShare); ok {
		return x.PublicKeyShare
	}
	return nil
}

func (x *Message) GetSecretShare() *SecretShare {
	if x, ok := x.GetPayload().(*Message_SecretShare); ok {
		return x.SecretShare
	}
	return nil
}

func (x *Message) GetBatchConfig() *BatchConfig {
	if x, ok := x.GetPayload().(*Message_BatchConfig); ok {
		return x.BatchConfig
	}
	return nil
}

type isMessage_Payload interface {
	isMessage_Payload()
}

type Message_PublicKeyCommitment struct {
	PublicKeyCommitment *PublicKeyCommitment `protobuf:"bytes,1,opt,name=public_key_commitment,json=publicKeyCommitment,proto3,oneof"`
}

type Message_PublicKeyShare struct {
	PublicKeyShare *PublicKeyShare `protobuf:"bytes,2,opt,name=public_key_share,json=publicKeyShare,proto3,oneof"`
}

type Message_SecretShare struct {
	SecretShare *SecretShare `protobuf:"bytes,3,opt,name=secret_share,json=secretShare,proto3,oneof"`
}

type Message_BatchConfig struct {
	BatchConfig *BatchConfig `protobuf:"bytes,4,opt,name=batch_config,json=batchConfig,proto3,oneof"`
}

func (*Message_PublicKeyCommitment) isMessage_Payload() {}

func (*Message_PublicKeyShare) isMessage_Payload() {}

func (*Message_SecretShare) isMessage_Payload() {}

func (*Message_BatchConfig) isMessage_Payload() {}

var File_shmsg_shmsg_proto protoreflect.FileDescriptor

var file_shmsg_shmsg_proto_rawDesc = []byte{
	0x0a, 0x11, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x2f, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x05, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x22, 0x55, 0x0a, 0x13, 0x50, 0x75,
	0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d, 0x65, 0x6e,
	0x74, 0x12, 0x1e, 0x0a, 0x0a, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0a, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x6e, 0x64, 0x65,
	0x78, 0x12, 0x1e, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d, 0x65, 0x6e, 0x74, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d, 0x65, 0x6e,
	0x74, 0x22, 0x3d, 0x0a, 0x0e, 0x50, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x53, 0x68,
	0x61, 0x72, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x64, 0x12, 0x10,
	0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6b, 0x65, 0x79,
	0x22, 0x47, 0x0a, 0x0b, 0x53, 0x65, 0x63, 0x72, 0x65, 0x74, 0x53, 0x68, 0x61, 0x72, 0x65, 0x12,
	0x1e, 0x0a, 0x0a, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x0a, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x12,
	0x18, 0x0a, 0x07, 0x70, 0x72, 0x69, 0x76, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x07, 0x70, 0x72, 0x69, 0x76, 0x6b, 0x65, 0x79, 0x22, 0x71, 0x0a, 0x0b, 0x42, 0x61, 0x74,
	0x63, 0x68, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x2a, 0x0a, 0x11, 0x73, 0x74, 0x61, 0x72,
	0x74, 0x5f, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x0f, 0x73, 0x74, 0x61, 0x72, 0x74, 0x42, 0x61, 0x74, 0x63, 0x68, 0x49,
	0x6e, 0x64, 0x65, 0x78, 0x12, 0x18, 0x0a, 0x07, 0x6b, 0x65, 0x79, 0x70, 0x65, 0x72, 0x73, 0x18,
	0x02, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x07, 0x6b, 0x65, 0x79, 0x70, 0x65, 0x72, 0x73, 0x12, 0x1c,
	0x0a, 0x09, 0x74, 0x68, 0x72, 0x65, 0x73, 0x68, 0x6f, 0x6c, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x09, 0x74, 0x68, 0x72, 0x65, 0x73, 0x68, 0x6f, 0x6c, 0x64, 0x22, 0x9b, 0x02, 0x0a,
	0x07, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x50, 0x0a, 0x15, 0x70, 0x75, 0x62, 0x6c,
	0x69, 0x63, 0x5f, 0x6b, 0x65, 0x79, 0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d, 0x65, 0x6e,
	0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x2e,
	0x50, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d,
	0x65, 0x6e, 0x74, 0x48, 0x00, 0x52, 0x13, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79,
	0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x6d, 0x65, 0x6e, 0x74, 0x12, 0x41, 0x0a, 0x10, 0x70, 0x75,
	0x62, 0x6c, 0x69, 0x63, 0x5f, 0x6b, 0x65, 0x79, 0x5f, 0x73, 0x68, 0x61, 0x72, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x2e, 0x50, 0x75, 0x62,
	0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x53, 0x68, 0x61, 0x72, 0x65, 0x48, 0x00, 0x52, 0x0e, 0x70,
	0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x53, 0x68, 0x61, 0x72, 0x65, 0x12, 0x37, 0x0a,
	0x0c, 0x73, 0x65, 0x63, 0x72, 0x65, 0x74, 0x5f, 0x73, 0x68, 0x61, 0x72, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x73, 0x68, 0x6d, 0x73, 0x67, 0x2e, 0x53, 0x65, 0x63, 0x72,
	0x65, 0x74, 0x53, 0x68, 0x61, 0x72, 0x65, 0x48, 0x00, 0x52, 0x0b, 0x73, 0x65, 0x63, 0x72, 0x65,
	0x74, 0x53, 0x68, 0x61, 0x72, 0x65, 0x12, 0x37, 0x0a, 0x0c, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x73,
	0x68, 0x6d, 0x73, 0x67, 0x2e, 0x42, 0x61, 0x74, 0x63, 0x68, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x48, 0x00, 0x52, 0x0b, 0x62, 0x61, 0x74, 0x63, 0x68, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x42,
	0x09, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x42, 0x09, 0x5a, 0x07, 0x2e, 0x3b,
	0x73, 0x68, 0x6d, 0x73, 0x67, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_shmsg_shmsg_proto_rawDescOnce sync.Once
	file_shmsg_shmsg_proto_rawDescData = file_shmsg_shmsg_proto_rawDesc
)

func file_shmsg_shmsg_proto_rawDescGZIP() []byte {
	file_shmsg_shmsg_proto_rawDescOnce.Do(func() {
		file_shmsg_shmsg_proto_rawDescData = protoimpl.X.CompressGZIP(file_shmsg_shmsg_proto_rawDescData)
	})
	return file_shmsg_shmsg_proto_rawDescData
}

var file_shmsg_shmsg_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_shmsg_shmsg_proto_goTypes = []interface{}{
	(*PublicKeyCommitment)(nil), // 0: shmsg.PublicKeyCommitment
	(*PublicKeyShare)(nil),      // 1: shmsg.PublicKeyShare
	(*SecretShare)(nil),         // 2: shmsg.SecretShare
	(*BatchConfig)(nil),         // 3: shmsg.BatchConfig
	(*Message)(nil),             // 4: shmsg.Message
}
var file_shmsg_shmsg_proto_depIdxs = []int32{
	0, // 0: shmsg.Message.public_key_commitment:type_name -> shmsg.PublicKeyCommitment
	1, // 1: shmsg.Message.public_key_share:type_name -> shmsg.PublicKeyShare
	2, // 2: shmsg.Message.secret_share:type_name -> shmsg.SecretShare
	3, // 3: shmsg.Message.batch_config:type_name -> shmsg.BatchConfig
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_shmsg_shmsg_proto_init() }
func file_shmsg_shmsg_proto_init() {
	if File_shmsg_shmsg_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_shmsg_shmsg_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PublicKeyCommitment); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_shmsg_shmsg_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PublicKeyShare); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_shmsg_shmsg_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SecretShare); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_shmsg_shmsg_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BatchConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_shmsg_shmsg_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Message); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_shmsg_shmsg_proto_msgTypes[4].OneofWrappers = []interface{}{
		(*Message_PublicKeyCommitment)(nil),
		(*Message_PublicKeyShare)(nil),
		(*Message_SecretShare)(nil),
		(*Message_BatchConfig)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_shmsg_shmsg_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_shmsg_shmsg_proto_goTypes,
		DependencyIndexes: file_shmsg_shmsg_proto_depIdxs,
		MessageInfos:      file_shmsg_shmsg_proto_msgTypes,
	}.Build()
	File_shmsg_shmsg_proto = out.File
	file_shmsg_shmsg_proto_rawDesc = nil
	file_shmsg_shmsg_proto_goTypes = nil
	file_shmsg_shmsg_proto_depIdxs = nil
}
