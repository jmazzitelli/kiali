package prometheus

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	utilcontext "github.com/kiali/kiali/util/context"
)

func TestQueryParamsRoundTripper_ThanosTenancyAutoDetect(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		expectParam bool
	}{
		{"thanos tenancy port", "https://thanos-querier.openshift-monitoring.svc.cluster.local:9092", true},
		{"thanos web port", "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091", false},
		{"standard prometheus", "http://prometheus.istio-system:9090", false},
		{"port 9092 but wrong host", "http://my-prometheus.default:9092", false},
		{"thanos host but no port", "https://thanos-querier.openshift-monitoring.svc.cluster.local/api", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var queryParams map[string]string
			if IsThanosTenancyURL(tc.url) {
				queryParams = map[string]string{"namespace": namespacePlaceholder}
			}
			rt := newQueryParamsRoundTripper(http.DefaultTransport, queryParams)
			if tc.expectParam {
				assert.IsType(t, &queryParamsRoundTripper{}, rt)
			} else {
				assert.Equal(t, http.DefaultTransport, rt)
			}
		})
	}
}

func TestQueryParamsRoundTripper_NamespaceResolution(t *testing.T) {
	mockRT := setupMockRoundTripper()
	params := map[string]string{"namespace": namespacePlaceholder}
	rt := newQueryParamsRoundTripper(mockRT, params)

	req, err := http.NewRequest("GET", "http://thanos:9092/api/v1/query?query=up", nil)
	assert.NoError(t, err)

	ctx := utilcontext.SetTenancyNamespace(context.Background(), "bookinfo")
	req = req.WithContext(ctx)

	resp, err := rt.RoundTrip(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, "bookinfo", mockRT.capturedRequest.URL.Query().Get("namespace"))
	assert.Equal(t, "up", mockRT.capturedRequest.URL.Query().Get("query"))
}

func TestQueryParamsRoundTripper_SkipsWhenNoNamespaceInContext(t *testing.T) {
	mockRT := setupMockRoundTripper()
	params := map[string]string{"namespace": namespacePlaceholder}
	rt := newQueryParamsRoundTripper(mockRT, params)

	req, err := http.NewRequest("GET", "http://thanos:9092/api/v1/query?query=up", nil)
	assert.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	assert.Empty(t, mockRT.capturedRequest.URL.Query().Get("namespace"))
}

func TestQueryParamsRoundTripper_PostToGetConversion(t *testing.T) {
	mockRT := setupMockRoundTripper()
	params := map[string]string{"namespace": namespacePlaceholder}
	rt := newQueryParamsRoundTripper(mockRT, params)

	body := strings.NewReader("query=up&time=1234")
	req, err := http.NewRequest("POST", "http://thanos:9092/api/v1/query", body)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := utilcontext.SetTenancyNamespace(context.Background(), "istio-system")
	req = req.WithContext(ctx)

	resp, err := rt.RoundTrip(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	captured := mockRT.capturedRequest
	assert.Equal(t, http.MethodGet, captured.Method)
	assert.Nil(t, captured.Body)
	assert.Equal(t, "up", captured.URL.Query().Get("query"))
	assert.Equal(t, "1234", captured.URL.Query().Get("time"))
	assert.Equal(t, "istio-system", captured.URL.Query().Get("namespace"))
}

func TestQueryParamsRoundTripper_NoopWithEmptyParams(t *testing.T) {
	rt := newQueryParamsRoundTripper(http.DefaultTransport, nil)
	assert.Equal(t, http.DefaultTransport, rt)

	rt = newQueryParamsRoundTripper(http.DefaultTransport, map[string]string{})
	assert.Equal(t, http.DefaultTransport, rt)
}
