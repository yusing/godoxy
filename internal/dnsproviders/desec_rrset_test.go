package dnsproviders

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nrdcg/desec"
)

// lego's desec provider PATCHes with desec.RRSet{Records: records} and no SubName.
// nrdcg/desec v0.11.2 dropped omitempty on subname, so that PATCH sends
// "subname":"" and deSEC rejects it as create-only.
func TestRRSetUpdateOmitsEmptySubName(t *testing.T) {
	body, err := json.Marshal(desec.RRSet{Records: []string{`"token"`}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"subname"`) {
		t.Fatalf("PATCH body must omit empty subname, got %s", body)
	}
}
