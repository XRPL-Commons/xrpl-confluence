package rpcclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rippled's account_info success response captured live during the F-phase
// smoke runs. Used to confirm the parser doesn't misclassify it as an error.
const accountInfoSuccessBody = `{"result":{"account_data":{"Account":"rJKuAjXsxgNmpXr19TvUmVKp4BBQz5LpMp","Balance":"9999999920","Flags":0,"LedgerEntryType":"AccountRoot","OwnerCount":4,"PreviousTxnID":"95A86F1AB890A214B77A66F81352A03351A1B6E4B16ECBE2A743CEAA11BF5A98","PreviousTxnLgrSeq":8,"Sequence":9,"index":"0694591233C9E4D896E2A02D35DA0263CCB220B66FE6D22D994585C4BC3D7536"},"account_flags":{"defaultRipple":false,"depositAuth":false,"disableMasterKey":false,"disallowIncomingXRP":false,"globalFreeze":false,"noFreeze":false},"ledger_hash":"ABC","ledger_index":140,"status":"success","validated":true}}`

// rippled's actNotFound response. The "error" / "status:error" / "error_code"
// fields all populate when the queried account doesn't exist on the queried
// ledger.
const accountInfoNotFoundBody = `{"result":{"account":"rDEFINITELYNOTREAL","error":"actNotFound","error_code":19,"error_message":"Account not found.","ledger_current_index":141,"request":{"account":"rDEFINITELYNOTREAL","command":"account_info"},"status":"error","validated":false}}`

// rippled returns this when the node isn't fully synced yet — a transient
// state we want to distinguish from genuine actNotFound.
const accountInfoNoNetworkBody = `{"result":{"error":"noNetwork","error_code":17,"error_message":"Not synced to the network.","request":{"command":"account_info"},"status":"error"}}`

func TestAccountInfo_SuccessParsesAccountData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(accountInfoSuccessBody))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	info, err := cl.AccountInfo("rJKuAjXsxgNmpXr19TvUmVKp4BBQz5LpMp")
	if err != nil {
		t.Fatalf("AccountInfo: %v", err)
	}
	if info.Account != "rJKuAjXsxgNmpXr19TvUmVKp4BBQz5LpMp" {
		t.Errorf("Account = %q", info.Account)
	}
	if info.Balance != "9999999920" {
		t.Errorf("Balance = %q", info.Balance)
	}
	if info.Sequence != 9 {
		t.Errorf("Sequence = %d, want 9", info.Sequence)
	}
}

func TestAccountInfo_ActNotFoundCarriesErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(accountInfoNotFoundBody))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	_, err := cl.AccountInfo("rDEFINITELYNOTREAL")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
	if !strings.Contains(err.Error(), "actNotFound") {
		t.Errorf("err = %v, want to contain 'actNotFound'", err)
	}
	if !strings.Contains(err.Error(), "Account not found") {
		t.Errorf("err = %v, want to contain rippled error_message", err)
	}
}

func TestAccountInfo_NoNetworkCarriesErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(accountInfoNoNetworkBody))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	_, err := cl.AccountInfo("rWHATEVER")
	if err == nil {
		t.Fatal("expected error when node not synced")
	}
	if !strings.Contains(err.Error(), "noNetwork") {
		t.Errorf("err = %v, want to contain 'noNetwork'", err)
	}
	if strings.Contains(err.Error(), "actNotFound") {
		t.Errorf("err = %v, must NOT misclassify noNetwork as actNotFound", err)
	}
}

func TestAccountInfo_EmptyResponseFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":{}}`))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	_, err := cl.AccountInfo("rWHATEVER")
	if err == nil {
		t.Fatal("expected error for empty result")
	}
	if !strings.Contains(err.Error(), "empty account_data") {
		t.Errorf("err = %v, want to contain 'empty account_data'", err)
	}
}
