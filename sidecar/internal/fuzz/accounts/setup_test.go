package accounts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

type capturedCall struct {
	TxType string
	IsIOU  bool
}

func newCaptureServer(results string) (*httptest.Server, *sync.Mutex, *[]capturedCall) {
	var mu sync.Mutex
	calls := []capturedCall{}
	var seq int64 = 100
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		method, _ := req["method"].(string)
		if method == "server_info" {
			mu.Lock()
			seq += 10 // advance by a lot per call so waitForValidation returns quickly
			cur := seq
			mu.Unlock()
			_, _ = w.Write([]byte(`{"result":{"info":{"server_state":"full","peers":1,"validated_ledger":{"seq":` + strconv.FormatInt(cur, 10) + `,"hash":"AAA"}}}}`))
			return
		}

		call := capturedCall{}
		if params, ok := req["params"].([]any); ok && len(params) > 0 {
			if p, ok := params[0].(map[string]any); ok {
				if txj, ok := p["tx_json"].(map[string]any); ok {
					call.TxType, _ = txj["TransactionType"].(string)
					if _, ok := txj["Amount"].(map[string]any); ok {
						call.IsIOU = true
					}
				}
			}
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()

		_, _ = w.Write([]byte(results))
	}))
	return srv, &mu, &calls
}

func TestSetupState_SubmitsMeshAndIOUFunding(t *testing.T) {
	srv, mu, calls := newCaptureServer(`{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"","tx_json":{"hash":"AAAA"},"status":"success"}}`)
	defer srv.Close()

	pool, err := NewPool(0xabc, 4)
	if err != nil {
		t.Fatal(err)
	}
	client := rpcclient.New(srv.URL)

	orig := setupPollInterval
	setupPollInterval = 1 * time.Millisecond
	defer func() { setupPollInterval = orig }()

	if err := SetupState(client, pool); err != nil {
		t.Fatalf("SetupState: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var trustSet, iouPay int
	for _, c := range *calls {
		switch {
		case c.TxType == "TrustSet":
			trustSet++
		case c.TxType == "Payment" && c.IsIOU:
			iouPay++
		}
	}
	if trustSet != 12 {
		t.Fatalf("TrustSet count = %d, want 12", trustSet)
	}
	if iouPay != 12 {
		t.Fatalf("IOU Payment count = %d, want 12", iouPay)
	}

	lastTrust, firstIOU := -1, -1
	for i, c := range *calls {
		if c.TxType == "TrustSet" {
			lastTrust = i
		}
		if c.TxType == "Payment" && c.IsIOU && firstIOU < 0 {
			firstIOU = i
		}
	}
	if !(lastTrust < firstIOU) {
		t.Fatalf("phase ordering violated: lastTrust=%d firstIOU=%d", lastTrust, firstIOU)
	}
}

func TestSetupState_FailsOnNonSuccess(t *testing.T) {
	srv, _, _ := newCaptureServer(`{"result":{"engine_result":"tecNO_LINE","engine_result_code":103,"engine_result_message":"no line","tx_json":{"hash":"X"},"status":"success"}}`)
	defer srv.Close()

	pool, _ := NewPool(0xabc, 3)
	client := rpcclient.New(srv.URL)

	orig := setupPollInterval
	setupPollInterval = 1 * time.Millisecond
	defer func() { setupPollInterval = orig }()

	if err := SetupState(client, pool); err == nil {
		t.Fatal("expected error on tecNO_LINE, got nil")
	} else if !strings.Contains(err.Error(), "tecNO_LINE") {
		t.Fatalf("error should mention engine result, got: %v", err)
	}
}

func TestSetupState_SkipsWhenPoolTooSmall(t *testing.T) {
	srv, _, calls := newCaptureServer(`{"result":{"engine_result":"tesSUCCESS","tx_json":{"hash":"X"},"status":"success"}}`)
	defer srv.Close()

	pool, _ := NewPool(0xabc, 1)
	if err := SetupState(rpcclient.New(srv.URL), pool); err != nil {
		t.Fatalf("SetupState on pool of 1: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("pool of 1 should submit 0 txs, got %d", len(*calls))
	}
}

func TestWaitForValidation_ReturnsAfterSeqAdvance(t *testing.T) {
	var callCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["method"] != "server_info" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		n := callCount.Add(1)
		// Return seq 100 on first 2 calls, 102 on 3rd onwards. Target advance of 2 → return at call 3.
		seq := 100
		if n >= 3 {
			seq = 102
		}
		payload := `{"result":{"info":{"server_state":"full","peers":1,"validated_ledger":{"seq":` +
			strconv.Itoa(seq) + `,"hash":"AAA"}}}}`
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	orig := setupPollInterval
	setupPollInterval = 5 * time.Millisecond
	defer func() { setupPollInterval = orig }()

	client := rpcclient.New(srv.URL)
	if err := waitForValidation(client, 2, 2*time.Second); err != nil {
		t.Fatalf("waitForValidation: %v", err)
	}
	if n := callCount.Load(); n < 3 {
		t.Fatalf("callCount = %d, expected at least 3 polls", n)
	}
}

func TestWaitForValidation_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"info":{"server_state":"full","peers":1,"validated_ledger":{"seq":100,"hash":"AAA"}}}}`))
	}))
	defer srv.Close()

	orig := setupPollInterval
	setupPollInterval = 5 * time.Millisecond
	defer func() { setupPollInterval = orig }()

	client := rpcclient.New(srv.URL)
	err := waitForValidation(client, 2, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
