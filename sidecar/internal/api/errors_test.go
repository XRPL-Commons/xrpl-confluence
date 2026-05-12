package api

import (
	"encoding/json"
	"testing"
)

func TestErrorResponseJSON(t *testing.T) {
	in := ErrorResponse{Error: Error{
		Code:    "scenario_invalid",
		Message: "workload.kind=replay requires reproducer.id",
		Field:   "workload.reproducer.id",
		Hint:    "set reproducer.id or change workload.kind",
	}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"error":{"code":"scenario_invalid","message":"workload.kind=replay requires reproducer.id","field":"workload.reproducer.id","hint":"set reproducer.id or change workload.kind"}}`
	if string(b) != want {
		t.Fatalf("got %s\nwant %s", b, want)
	}

	var rt ErrorResponse
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rt != in {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", rt, in)
	}
}

func TestErrorOmitsEmptyOptionalFields(t *testing.T) {
	b, _ := json.Marshal(Error{Code: "bad", Message: "x"})
	const want = `{"code":"bad","message":"x"}`
	if string(b) != want {
		t.Fatalf("got %s\nwant %s", b, want)
	}
}
