package prometheus

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kiali/kiali/log"
	utilcontext "github.com/kiali/kiali/util/context"
)

const namespacePlaceholder = "{namespace}"

// IsThanosTenancyURL reports whether promURL targets the OpenShift Thanos
// Querier tenancy port (9092). Code that constructs Prometheus queries should
// use this to choose the appropriate query strategy: the tenancy port requires
// a concrete namespace= URL parameter on every request and does not support
// multi-namespace label matchers (e.g. regex selectors spanning namespaces),
// whereas standard Prometheus endpoints and the Thanos web port (9091) have no
// such restriction and can aggregate across namespaces in a single query.
func IsThanosTenancyURL(promURL string) bool {
	return strings.Contains(promURL, "thanos-querier.openshift-monitoring") &&
		strings.Contains(promURL, ":9092")
}

// queryParamsRoundTripper appends URL query parameters to every outgoing
// Prometheus HTTP request and converts POST requests to GET. Parameter values
// may contain the placeholder {namespace} which is resolved at request time
// from the context (set via utilcontext.SetTenancyNamespace). This allows
// Kiali to dynamically scope each Prometheus request to the namespace being
// queried, which is required by the OpenShift Thanos Querier tenancy port
// (9092). The POST→GET conversion is necessary because kube-rbac-proxy only
// extracts the namespace query parameter from GET request URLs.
type queryParamsRoundTripper struct {
	originalRT http.RoundTripper
	params     map[string]string
}

func (rt *queryParamsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	q := req.URL.Query()

	if req.Method == http.MethodPost && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		bodyParams, _ := url.ParseQuery(string(body))
		for k, vals := range bodyParams {
			for _, v := range vals {
				q.Set(k, v)
			}
		}
		req.Method = http.MethodGet
		req.Body = nil
		req.ContentLength = 0
		req.Header.Del("Content-Type")
	}

	tenancyNs := utilcontext.GetTenancyNamespace(req.Context())
	for k, v := range rt.params {
		if strings.Contains(v, namespacePlaceholder) {
			if tenancyNs == "" {
				continue
			}
			v = strings.ReplaceAll(v, namespacePlaceholder, tenancyNs)
		}
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	if log.IsTrace() {
		log.Tracef("queryParamsRoundTripper: %s %s", req.Method, req.URL.String())
	}
	return rt.originalRT.RoundTrip(req)
}

func newQueryParamsRoundTripper(rt http.RoundTripper, params map[string]string) http.RoundTripper {
	if len(params) == 0 {
		return rt
	}
	return &queryParamsRoundTripper{
		originalRT: rt,
		params:     params,
	}
}
