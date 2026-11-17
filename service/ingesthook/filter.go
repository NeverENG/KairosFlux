// Package ingesthook validates writes at the Kair and gRPC ingress boundaries.
package ingesthook

import (
	"github.com/ChronoBrew/KairosFlux/internal/metrics"
	"github.com/ChronoBrew/KairosFlux/kairnet"
	"github.com/ChronoBrew/KairosFlux/proto"
	"github.com/ChronoBrew/KairosFlux/service/ingesthook/schema"
)

// Filter checks frame shape, value size and registered schemas; it never rewrites values.
type Filter struct {
	maxValueLen int
}

// NewFilter constructs a filter; maxValueLen <= 0 disables the size limit.
func NewFilter(maxValueLen int) *Filter { return &Filter{maxValueLen: maxValueLen} }

// Handle validates a v1 PUT frame before the router writes it.
func (f *Filter) Handle(req kairnet.Request) (kairnet.HookAction, string) {
	if req.MsgID() != proto.MsgPut {
		return kairnet.HookPass, ""
	}
	key, value, ok := proto.DecodePutFrame(req.MsgData())
	if !ok {
		metrics.FramesDroppedMalformed.Add(1)
		return kairnet.HookDrop, "malformed_frame"
	}
	_, _, result, _, reason := f.Validate(key, value)
	if result == ResultDrop {
		return kairnet.HookDrop, reason
	}
	return kairnet.HookPass, ""
}

// Result is the disposition of a write.
type Result int

const (
	ResultPass Result = iota
	ResultDrop
)

// Validate checks size and the schema selected by key prefix. The unchanged value and
// changed=false are retained for callers of the existing write API.
func (f *Filter) Validate(key, value []byte) (newValue []byte, changed bool, result Result, code uint16, reason string) {
	return f.ValidateForType(0, key, value)
}

// ValidateVersioned uses the same checks; repeated versions do not imply a bad timestamp.
func (f *Filter) ValidateVersioned(key, value []byte) (newValue []byte, changed bool, result Result, code uint16, reason string) {
	return f.Validate(key, value)
}

// ValidateForType uses the declared type when nonzero, otherwise the key prefix.
// Unknown declared types are rejected instead of falling back to the key prefix.
func (f *Filter) ValidateForType(typeID uint16, key, value []byte) (newValue []byte, changed bool, result Result, code uint16, reason string) {
	if f.maxValueLen > 0 && len(value) > f.maxValueLen {
		metrics.FramesDroppedOversized.Add(1)
		return value, false, ResultDrop, 0, "oversized_value"
	}
	if typeID != 0 {
		descriptor, ok := schema.LookupByType(typeID)
		if !ok {
			metrics.FramesDroppedSchema.Add(1)
			return value, false, ResultDrop, 0, "unknown_declared_type"
		}
		if err := descriptor.Validation.Validate(value); err != nil {
			metrics.FramesDroppedSchema.Add(1)
			return value, false, ResultDrop, schema.CodeOf(err, 0), err.Error()
		}
	} else if descriptor, ok := schema.LookupDescriptor(key); ok {
		if err := descriptor.Validation.Validate(value); err != nil {
			metrics.FramesDroppedSchema.Add(1)
			return value, false, ResultDrop, schema.CodeOf(err, 0), err.Error()
		}
	}
	return value, false, ResultPass, 0, ""
}

// ValidateVersionedForType applies the same type and size checks to versioned writes.
func (f *Filter) ValidateVersionedForType(typeID uint16, key, value []byte) (newValue []byte, changed bool, result Result, code uint16, reason string) {
	return f.ValidateForType(typeID, key, value)
}
