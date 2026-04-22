package generator

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func TestDiscoverEnabledAmendments_ParsesFeatureRPC(t *testing.T) {
	body := `{"result":{"features":{
		"HASH_A":{"name":"fixUniversalNumber","enabled":true,"supported":true},
		"HASH_B":{"name":"AMM","enabled":true,"supported":true},
		"HASH_C":{"name":"NotYet","enabled":false,"supported":true}
	},"status":"success"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := rpcclient.New(srv.URL)
	got, err := DiscoverEnabledAmendments(client)
	if err != nil {
		t.Fatalf("DiscoverEnabledAmendments: %v", err)
	}
	sort.Strings(got)
	want := []string{"AMM", "fixUniversalNumber"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiscoverEnabledAmendments_EmptyOnEmptyFeatures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"features":{},"status":"success"}}`))
	}))
	defer srv.Close()

	got, err := DiscoverEnabledAmendments(rpcclient.New(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
