package benchmarks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

var benchClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 100,
	},
}

func drainAndCloseBody(b *testing.B, body io.ReadCloser) {
	b.Helper()
	if _, err := io.Copy(io.Discard, body); err != nil {
		b.Fatalf("failed to drain response body: %v", err)
	}
	if err := body.Close(); err != nil {
		b.Fatalf("failed to close response body: %v", err)
	}
}

func BenchmarkHealthEndpoint(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := benchClient.Get(benchServer.URL + "/health")
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkAuthGoogleInitiation(b *testing.B) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(benchServer.URL + "/v1/auth/google")
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusFound {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkAuthMicrosoftInitiation(b *testing.B) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(benchServer.URL + "/v1/auth/microsoft")
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusFound {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkGetMe(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkGetUser(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/users/"+benchUserID, nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkListOrganizations(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/organizations", nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkCreateOrganization(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		body := map[string]string{
			"name": fmt.Sprintf("Bench Org %d", i),
			"slug": fmt.Sprintf("bench-org-%d", i),
		}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", benchServer.URL+"/v1/organizations", bytes.NewBuffer(jsonBody))
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		req.Header.Set("Content-Type", "application/json")
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkGetOrganization(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/organizations/"+benchOrgID, nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkListEvents(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/events", nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkCreateEvent(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		body := map[string]interface{}{
			"location":    "Bench Location",
			"description": "Bench Description",
			"org_id":      benchOrgID,
		}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", benchServer.URL+"/v1/events", bytes.NewBuffer(jsonBody))
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		req.Header.Set("Content-Type", "application/json")
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}

func BenchmarkGetEvent(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", benchServer.URL+"/v1/events/"+benchEventID, nil)
		req.AddCookie(&http.Cookie{Name: "capy_auth", Value: benchJWTToken})
		resp, err := benchClient.Do(req)
		if err != nil {
			b.Fatalf("failed to make request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
		drainAndCloseBody(b, resp.Body)
	}
}
