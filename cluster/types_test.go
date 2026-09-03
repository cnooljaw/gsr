package cluster

import (
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const clusterTestCommand runtime.CommandID = 0x05000101

// 验证 Request、Response 和 Error 三类 ClusterEnvelope 遵守 Session、Target、Payload 契约。
func TestClusterEnvelopeValidateFrames(t *testing.T) {
	cases := []struct {
		name     string
		envelope ClusterEnvelope
		wantErr  bool
	}{
		{
			name: "send request by id",
			envelope: ClusterEnvelope{
				Kind:    FrameRequest,
				Target:  ServiceTarget{ServiceID: 7},
				Command: clusterTestCommand,
			},
		},
		{
			name: "call request by name",
			envelope: ClusterEnvelope{
				Kind:    FrameRequest,
				Session: 9,
				Target:  ServiceTarget{ServiceName: "room"},
				Command: clusterTestCommand,
			},
		},
		{
			name: "response",
			envelope: ClusterEnvelope{
				Kind:    FrameResponse,
				Session: 9,
				Target:  ServiceTarget{ServiceID: 7},
				Command: clusterTestCommand,
				Payload: []byte("ok"),
			},
		},
		{
			name: "error",
			envelope: ClusterEnvelope{
				Kind:    FrameError,
				Session: 9,
				Target:  ServiceTarget{ServiceID: 7},
				Command: clusterTestCommand,
				Error:   runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeServiceNotFound, "not found"),
			},
		},
		{
			name: "both target forms",
			envelope: ClusterEnvelope{
				Kind:    FrameRequest,
				Target:  ServiceTarget{ServiceID: 7, ServiceName: "room"},
				Command: clusterTestCommand,
			},
			wantErr: true,
		},
		{
			name: "response without session",
			envelope: ClusterEnvelope{
				Kind:    FrameResponse,
				Target:  ServiceTarget{ServiceID: 7},
				Command: clusterTestCommand,
			},
			wantErr: true,
		},
		{
			name: "error with payload",
			envelope: ClusterEnvelope{
				Kind:    FrameError,
				Session: 9,
				Target:  ServiceTarget{ServiceID: 7},
				Command: clusterTestCommand,
				Payload: []byte("unexpected"),
				Error:   runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeServiceNotFound, "not found"),
			},
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.envelope.Validate()
			if testCase.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

// 验证 ClusterEnvelope 的 Clone 隔离 Payload 和 StatusError，避免 Transport 共享可变值。
func TestClusterEnvelopeCloneOwnsMutableValues(t *testing.T) {
	original := ClusterEnvelope{
		Kind:    FrameError,
		Session: 1,
		Target:  ServiceTarget{ServiceName: "room"},
		Command: clusterTestCommand,
		Error:   runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeServiceNotFound, "not found"),
		Payload: nil,
	}
	clone := original.Clone()
	clone.Error.Message = "changed"
	if clone.Error == original.Error {
		t.Fatal("Clone() reused Error pointer")
	}
	if original.Error.Message != "not found" {
		t.Fatalf("original Error.Message = %q, want unchanged", original.Error.Message)
	}

	original.Kind = FrameResponse
	original.Error = nil
	original.Payload = []byte("before")
	clone = original.Clone()
	clone.Payload[0] = 'a'
	if string(original.Payload) != "before" {
		t.Fatalf("original Payload = %q, want unchanged", original.Payload)
	}
}
