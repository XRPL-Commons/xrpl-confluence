package oracle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

func metaServer(affected string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"result":{"meta":{"TransactionResult":"tesSUCCESS","AffectedNodes":` + affected + `},"validated":true}}`
		_, _ = w.Write([]byte(body))
	}))
}

func TestCompareTxMetadata_AgreementAcrossNodes(t *testing.T) {
	const af = `[{"CreatedNode":{"LedgerEntryType":"Offer","LedgerIndex":"A"}}]`
	srvA := metaServer(af)
	defer srvA.Close()
	srvB := metaServer(af)
	defer srvB.Close()

	nodes := []Node{
		{Name: "A", Client: rpcclient.New(srvA.URL)},
		{Name: "B", Client: rpcclient.New(srvB.URL)},
	}
	o := New(nodes)

	cmp := o.CompareTxMetadata(context.Background(), "HASH")
	if !cmp.Agreed {
		t.Fatalf("want agreement, got %+v", cmp)
	}
	if len(cmp.NodeMeta) != 2 {
		t.Fatalf("want 2 NodeMeta, got %d", len(cmp.NodeMeta))
	}
}

func TestCompareTxMetadata_Divergence(t *testing.T) {
	srvA := metaServer(`[{"CreatedNode":{"LedgerEntryType":"Offer","LedgerIndex":"A"}}]`)
	defer srvA.Close()
	srvB := metaServer(`[{"CreatedNode":{"LedgerEntryType":"Offer","LedgerIndex":"B"}}]`)
	defer srvB.Close()

	nodes := []Node{
		{Name: "A", Client: rpcclient.New(srvA.URL)},
		{Name: "B", Client: rpcclient.New(srvB.URL)},
	}
	o := New(nodes)

	cmp := o.CompareTxMetadata(context.Background(), "HASH")
	if cmp.Agreed {
		t.Fatalf("expected divergence, got %+v", cmp)
	}
	if len(cmp.NodeMeta) != 2 {
		t.Fatalf("expected both nodes reported, got %d", len(cmp.NodeMeta))
	}
	var a, b []map[string]any
	if err := json.Unmarshal(cmp.NodeMeta[0].AffectedNodes, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(cmp.NodeMeta[1].AffectedNodes, &b); err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("bad shapes: a=%d b=%d", len(a), len(b))
	}
}
