package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestChromeLaunchArgs_ForcesLocalhostThroughProxy(t *testing.T) {
	args := chromeLaunchArgs("127.0.0.1:8888", "/tmp/profile", "http://localhost:3001")
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "--proxy-server=http://127.0.0.1:8888") {
		t.Fatalf("Chrome needs a manual proxy (PAC cannot proxy localhost), got:\n%s", joined)
	}
	if strings.Contains(joined, "--proxy-pac-url") {
		t.Fatalf("PAC must not be used; Chrome cannot subtract loopback bypass for PAC, got:\n%s", joined)
	}
	if !strings.Contains(joined, "--proxy-bypass-list=<-loopback>;127.0.0.1:4488") {
		t.Fatalf("Chrome must proxy localhost except apilens ui :4488, got:\n%s", joined)
	}
	if args[len(args)-1] != "http://localhost:3001" {
		t.Fatalf("open URL = %q", args[len(args)-1])
	}
}

func TestFormatExchangeLine_ShowsGraphQLOperation(t *testing.T) {
	line := formatExchangeLine(domain.Exchange{
		Display: 15,
		Request: domain.HTTPRequest{
			Method: "POST",
			URL:    "http://localhost:3000/graphql",
			Body:   []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`),
		},
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"ping":{}}}`)},
		Timing:   domain.Timing{Duration: 247 * time.Millisecond},
	})
	if !strings.Contains(line, "QUERY") || !strings.Contains(line, "Ping") {
		t.Fatalf("expected QUERY Ping, got %q", line)
	}
	if strings.Contains(line, "/graphql") {
		t.Fatalf("should not print the HTTP URL for GraphQL, got %q", line)
	}
}

func TestFormatExchangeLine_MarksGraphQLErrors(t *testing.T) {
	line := formatExchangeLine(domain.Exchange{
		Display: 1,
		Request: domain.HTTPRequest{
			Method: "POST",
			URL:    "http://localhost:3000/graphql",
			Body:   []byte(`{"query":"{ ping { message } }"}`),
		},
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"errors":[{"message":"nope"}]}`)},
	})
	if !strings.Contains(line, "200*") {
		t.Fatalf("expected 200* for GraphQL errors, got %q", line)
	}
}
