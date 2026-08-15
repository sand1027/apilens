package cli

import (
	"strings"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func gqlEx(query string, status int) domain.Exchange {
	return domain.Exchange{
		Display:  1,
		Request:  domain.HTTPRequest{Method: "POST", URL: "http://localhost:3000/graphql", Body: []byte(query)},
		Response: domain.HTTPResponse{StatusCode: status},
	}
}

func TestWatchLineState_CollapsesConsecutiveDuplicates(t *testing.T) {
	q := `{"query":"query Users { users { id } }","operationName":"Users"}`
	var st watchLineState
	var lines []string
	for i := 0; i < 3; i++ {
		ex := gqlEx(q, 200)
		ex.Display = domain.DisplayID(i + 1)
		lines = append(lines, st.push(ex)...)
	}
	lines = append(lines, st.flush()...)
	joined := strings.Join(lines, "\n")
	if strings.Count(joined, "QUERY") != 1 {
		t.Errorf("expected one QUERY line, got:\n%s", joined)
	}
	if !strings.Contains(joined, "(×3 same)") {
		t.Errorf("expected collapse marker, got:\n%s", joined)
	}
}

func TestWatchLineState_DifferentOpsStaySeparate(t *testing.T) {
	var st watchLineState
	var lines []string
	lines = append(lines, st.push(gqlEx(`{"query":"query Users { users { id } }","operationName":"Users"}`, 200))...)
	lines = append(lines, st.push(gqlEx(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`, 200))...)
	lines = append(lines, st.flush()...)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Users") || !strings.Contains(joined, "Ping") {
		t.Errorf("expected both ops, got:\n%s", joined)
	}
	if strings.Contains(joined, "same") {
		t.Errorf("did not expect collapse, got:\n%s", joined)
	}
}
