// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dag

import (
	"errors"
	"io/ioutil"
	"testing"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryPolicyIngress(t *testing.T) {
	tests := map[string]struct {
		i    *networking_v1.Ingress
		want *RetryPolicy
	}{
		"no anotations": {
			i:    &networking_v1.Ingress{},
			want: nil,
		},
		"retry-on": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on": "5xx",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly disabled retries": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "-1",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 0,
			},
		},
		"num-retries": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "7",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 7,
			},
		},
		"no retry count, per try timeout": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "10s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"explicit 0s timeout": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "0s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ingressRetryPolicy(tc.i, &logrus.Logger{Out: ioutil.Discard})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRetryPolicy(t *testing.T) {
	tests := map[string]struct {
		rp   *contour_api_v1.RetryPolicy
		want *RetryPolicy
	}{
		"nil retry policy": {
			rp:   nil,
			want: nil,
		},
		"empty policy": {
			rp: &contour_api_v1.RetryPolicy{},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly zero retries": {
			rp: &contour_api_v1.RetryPolicy{
				NumRetries: 0, // zero value for NumRetries
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"no retry count, per try timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "10s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"explicit 0s timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "0s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
		"retry on": {
			rp: &contour_api_v1.RetryPolicy{
				RetryOn: []contour_api_v1.RetryOn{"gateway-error", "connect-failure"},
			},
			want: &RetryPolicy{
				RetryOn:    "gateway-error,connect-failure",
				NumRetries: 1,
			},
		},
		"retriable status codes": {
			rp: &contour_api_v1.RetryPolicy{
				RetriableStatusCodes: []uint32{502, 503, 504},
			},
			want: &RetryPolicy{
				RetryOn:              "5xx",
				RetriableStatusCodes: []uint32{502, 503, 504},
				NumRetries:           1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retryPolicy(tc.rp)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTimeoutPolicy(t *testing.T) {
	tests := map[string]struct {
		tp      *contour_api_v1.TimeoutPolicy
		want    TimeoutPolicy
		wantErr bool
	}{
		"nil timeout policy": {
			tp:   nil,
			want: TimeoutPolicy{},
		},
		"empty timeout policy": {
			tp:   &contour_api_v1.TimeoutPolicy{},
			want: TimeoutPolicy{},
		},
		"valid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "1m30s",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DurationSetting(90 * time.Second),
			},
		},
		"invalid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "90", // 90 what?
			},
			wantErr: true,
		},
		"infinite response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "infinite",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DisabledSetting(),
			},
		},
		"idle timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Idle: "900s",
			},
			want: TimeoutPolicy{
				IdleTimeout: timeout.DurationSetting(900 * time.Second),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := timeoutPolicy(tc.tp)
			if tc.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, tc.want, got)
				assert.NoError(t, gotErr)
			}

		})
	}
}

func TestLoadBalancerPolicy(t *testing.T) {
	tests := map[string]struct {
		lbp  *contour_api_v1.LoadBalancerPolicy
		want string
	}{
		"nil": {
			lbp:  nil,
			want: "",
		},
		"empty": {
			lbp:  &contour_api_v1.LoadBalancerPolicy{},
			want: "",
		},
		"WeightedLeastRequest": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "WeightedLeastRequest",
			},
			want: "WeightedLeastRequest",
		},
		"Random": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Random",
			},
			want: "Random",
		},
		"Cookie": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			want: "Cookie",
		},
		"RequestHash": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
			want: "RequestHash",
		},
		"unknown": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "please",
			},
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := loadBalancerPolicy(tc.lbp)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHeadersPolicy(t *testing.T) {
	tests := map[string]struct {
		hp      *contour_api_v1.HeadersPolicy
		dhp     HeadersPolicy
		want    HeadersPolicy
		wantErr bool
	}{
		"no percentage unchanged": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-App-Weight",
					Value: "100",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "100",
				},
			},
		},
		"simple percentage escape": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-App-Weight",
					Value: "100%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "100%%",
				},
			},
		},
		"known good Envoy dynamic header unescaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Envoy-Hostname",
					Value: "%HOSTNAME%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Hostname": "%HOSTNAME%",
				},
			},
		},
		"unknown Envoy dynamic header is escaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Envoy-Unknown",
					Value: "%UNKNOWN%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Unknown": "%%UNKNOWN%%",
				},
			},
		},
		"valid Envoy REQ header unescaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Request-Host",
					Value: "%REQ(Host)%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Request-Host": "%REQ(Host)%",
				},
			},
		},
		"invalid Envoy REQ header is escaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Request-Host",
					Value: "%REQ(inv@lid-header)%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Request-Host": "%%REQ(inv@lid-header)%%",
				},
			},
		},
		"header value with dynamic and non-dynamic content and multiple dynamic fields": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Host-Protocol",
					Value: "%HOSTNAME% - %PROTOCOL%",
				}},
			},
			dhp: HeadersPolicy{},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Host-Protocol": "%HOSTNAME% - %PROTOCOL%",
				},
			},
		},
		"dynamic service headers": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "l5d-dst-override",
					Value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"L5d-Dst-Override": "myservice.myns.svc.cluster.local:80",
				},
			},
		},
		"default header value with different object header value combined": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Host-Protocol",
					Value: "%HOSTNAME% - %PROTOCOL%",
				}},
			},
			dhp: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Hostname": "%HOSTNAME%",
				},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Hostname": "%HOSTNAME%",
					"X-Host-Protocol":  "%HOSTNAME% - %PROTOCOL%",
				},
			},
		},
		"default header value with same object header value not replaced": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-App-Weight",
					Value: "100",
				}},
			},
			dhp: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "10",
				},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "100",
				},
			},
		},
		"same header removed in default and object": {
			hp: &contour_api_v1.HeadersPolicy{
				Remove: []string{"X-Sensitive-Header"},
			},
			dhp: HeadersPolicy{
				Remove: []string{"X-Sensitive-Header"},
			},
			want: HeadersPolicy{
				Set:    map[string]string{},
				Remove: []string{"X-Sensitive-Header"},
			},
		},
		"default headers with nil object headers": {
			hp: nil,
			dhp: HeadersPolicy{
				Remove: []string{"X-Sensitive-Header"},
			},
			want: HeadersPolicy{
				Set:    map[string]string{},
				Remove: []string{"X-Sensitive-Header"},
			},
		},
	}

	dynamicHeaders := map[string]string{
		"CONTOUR_NAMESPACE":    "myns",
		"CONTOUR_SERVICE_NAME": "myservice",
		"CONTOUR_SERVICE_PORT": "80",
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := headersPolicyService(&tc.dhp, tc.hp, dynamicHeaders)
			if tc.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, tc.want, *got)
				assert.NoError(t, gotErr)
			}
		})
	}
}

func TestRateLimitPolicy(t *testing.T) {
	tests := map[string]struct {
		in      *contour_api_v1.RateLimitPolicy
		want    *RateLimitPolicy
		wantErr string
	}{
		"nil input": {
			in:   nil,
			want: nil,
		},
		"nil local rate limit policy": {
			in:   &contour_api_v1.RateLimitPolicy{},
			want: nil,
		},
		"local - no burst": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 3,
					Unit:     "second",
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     3,
					TokensPerFill: 3,
					FillInterval:  time.Second,
				},
			},
		},
		"local - burst": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 3,
					Unit:     "second",
					Burst:    4,
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     7,
					TokensPerFill: 3,
					FillInterval:  time.Second,
				},
			},
		},
		"local - custom response status code": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests:           10,
					Unit:               "minute",
					ResponseStatusCode: 431,
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:          10,
					TokensPerFill:      10,
					FillInterval:       time.Minute,
					ResponseStatusCode: 431,
				},
			},
		},
		"local - custom response headers to add": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "hour",
					ResponseHeadersToAdd: []contour_api_v1.HeaderValue{
						{
							Name:  "header-1",
							Value: "header-value-1",
						},
						{
							Name:  "header-2",
							Value: "header-value-2",
						},
					},
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     10,
					TokensPerFill: 10,
					FillInterval:  time.Hour,
					ResponseHeadersToAdd: map[string]string{
						"Header-1": "header-value-1",
						"Header-2": "header-value-2",
					},
				},
			},
		},
		"local - duplicate response header": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "hour",
					ResponseHeadersToAdd: []contour_api_v1.HeaderValue{
						{
							Name:  "duplicate-header",
							Value: "header-value-1",
						},
						{
							Name:  "duplicate-header",
							Value: "header-value-2",
						},
					},
				},
			},
			wantErr: "duplicate header addition: \"Duplicate-Header\"",
		},
		"local - invalid response header name": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "hour",
					ResponseHeadersToAdd: []contour_api_v1.HeaderValue{
						{
							Name:  "invalid-header!",
							Value: "header-value-1",
						},
					},
				},
			},
			wantErr: `invalid header name "Invalid-Header!": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`,
		},
		"local - invalid unit": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "invalid-unit",
				},
			},
			wantErr: "invalid unit \"invalid-unit\" in local rate limit policy",
		},
		"local - invalid requests": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 0,
					Unit:     "second",
				},
			},
			wantErr: "invalid requests value 0 in local rate limit policy",
		},
		"global - multiple descriptors": {
			in: &contour_api_v1.RateLimitPolicy{
				Global: &contour_api_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_api_v1.RateLimitDescriptor{
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_api_v1.GenericKeyDescriptor{
										Key:   "generic-key-key",
										Value: "generic-key-value",
									},
								},
								{
									RemoteAddress: &contour_api_v1.RemoteAddressDescriptor{},
								},
								{
									RequestHeader: &contour_api_v1.RequestHeaderDescriptor{
										HeaderName:    "X-Header",
										DescriptorKey: "request-header-key",
									},
								},
							},
						},
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{
									RemoteAddress: &contour_api_v1.RemoteAddressDescriptor{},
								},
								{
									GenericKey: &contour_api_v1.GenericKeyDescriptor{
										Key:   "generic-key-key-2",
										Value: "generic-key-value-2",
									},
								},
							},
						},
					},
				},
			},
			want: &RateLimitPolicy{
				Global: &GlobalRateLimitPolicy{
					Descriptors: []*RateLimitDescriptor{
						{
							Entries: []RateLimitDescriptorEntry{
								{
									GenericKey: &GenericKeyDescriptorEntry{
										Key:   "generic-key-key",
										Value: "generic-key-value",
									},
								},
								{
									RemoteAddress: &RemoteAddressDescriptorEntry{},
								},
								{
									HeaderMatch: &HeaderMatchDescriptorEntry{
										HeaderName: "X-Header",
										Key:        "request-header-key",
									},
								},
							},
						},
						{
							Entries: []RateLimitDescriptorEntry{
								{
									RemoteAddress: &RemoteAddressDescriptorEntry{},
								},
								{
									GenericKey: &GenericKeyDescriptorEntry{
										Key:   "generic-key-key-2",
										Value: "generic-key-value-2",
									},
								},
							},
						},
					},
				},
			},
		},
		"global - multiple descriptor entries set": {
			in: &contour_api_v1.RateLimitPolicy{
				Global: &contour_api_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_api_v1.RateLimitDescriptor{
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{
									GenericKey:    &contour_api_v1.GenericKeyDescriptor{},
									RemoteAddress: &contour_api_v1.RemoteAddressDescriptor{},
								},
							},
						},
					},
				},
			},
			wantErr: "rate limit descriptor entry must have exactly one field set",
		},
		"global - no descriptor entries set": {
			in: &contour_api_v1.RateLimitPolicy{
				Global: &contour_api_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_api_v1.RateLimitDescriptor{
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{},
							},
						},
					},
				},
			},
			wantErr: "rate limit descriptor entry must have exactly one field set",
		},
		"global - header value match": {
			in: &contour_api_v1.RateLimitPolicy{
				Global: &contour_api_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_api_v1.RateLimitDescriptor{
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{
									RequestHeaderValueMatch: &contour_api_v1.RequestHeaderValueMatchDescriptor{
										Headers: []contour_api_v1.HeaderMatchCondition{
											{
												Name:       "X-Header",
												NotPresent: true,
											},
										},
										ExpectMatch: true,
										Value:       "header-is-not-present",
									},
								},
							},
						},
					},
				},
			},
			want: &RateLimitPolicy{
				Global: &GlobalRateLimitPolicy{
					Descriptors: []*RateLimitDescriptor{
						{
							Entries: []RateLimitDescriptorEntry{
								{
									HeaderValueMatch: &HeaderValueMatchDescriptorEntry{
										Headers: []HeaderMatchCondition{
											{
												Name:      "X-Header",
												MatchType: "present",
												Invert:    true,
											},
										},
										ExpectMatch: true,
										Value:       "header-is-not-present",
									},
								},
							},
						},
					},
				},
			},
		},
		"global and local": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 20,
					Unit:     "second",
				},
				Global: &contour_api_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_api_v1.RateLimitDescriptor{
						{
							Entries: []contour_api_v1.RateLimitDescriptorEntry{
								{
									RemoteAddress: &contour_api_v1.RemoteAddressDescriptor{},
								},
							},
						},
					},
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     20,
					TokensPerFill: 20,
					FillInterval:  time.Second,
				},
				Global: &GlobalRateLimitPolicy{
					Descriptors: []*RateLimitDescriptor{
						{
							Entries: []RateLimitDescriptorEntry{
								{
									RemoteAddress: &RemoteAddressDescriptorEntry{},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rlp, err := rateLimitPolicy(tc.in)

			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.Equal(t, tc.want, rlp)
			}
		})
	}
}

func TestValidateHeaderAlteration(t *testing.T) {
	tests := []struct {
		name    string
		in      *contour_api_v1.HeadersPolicy
		dyn     map[string]string
		dhp     *HeadersPolicy
		want    *HeadersPolicy
		wantErr error
	}{{
		name: "empty is fine",
	}, {
		name: "set two, remove one",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "bar",
			}, {
				Name:  "k-baz", // This gets canonicalized
				Value: "blah",
			}},
			Remove: []string{"K-Nada"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "bar",
				"K-Baz": "blah",
			},
			Remove: []string{"K-Nada"},
		},
	}, {
		name: "duplicate set",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "bar",
			}, {
				Name:  "k-foo", // This gets canonicalized
				Value: "blah",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`duplicate header addition: "K-Foo"`),
	}, {
		name: "duplicate remove",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"K-Foo", "k-foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`duplicate header removal: "K-Foo"`),
	}, {
		name: "invalid set header",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "  K-Foo",
				Value: "bar",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`invalid set header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid set default header",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"  K-Foo": "bar",
			},
		},
		wantErr: errors.New(`invalid set header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid remove header",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`invalid remove header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid remove default header",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		wantErr: errors.New(`invalid remove header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid set header (special headers)",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "Host",
				Value: "bar",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`rewriting "Host" header is not supported`),
	}, {
		name: "invalid set default header (special headers)",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "ook?",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"Host": "bar",
			},
		},
		wantErr: errors.New(`rewriting "Host" header is not supported`),
	}, {
		name: "percents are escaped",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}, {
				Name:  "Lot-Of-Percents",
				Value: "%%%%%",
			}, {
				Name:  "k-baz",                      // This gets canonicalized
				Value: "%DOWNSTREAM_LOCAL_ADDRESS%", // This is a known Envoy dynamic header
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo":           "100%%",
				"K-Baz":           "%DOWNSTREAM_LOCAL_ADDRESS%",
				"Lot-Of-Percents": "%%%%%%%%%%",
			},
		},
	}, {
		name: "dynamic service headers",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "l5d-dst-override",
				Value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE":    "myns",
			"CONTOUR_SERVICE_NAME": "myservice",
			"CONTOUR_SERVICE_PORT": "80",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"L5d-Dst-Override": "myservice.myns.svc.cluster.local:80",
			},
		},
	}, {
		name: "dynamic service headers without service name and port",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "l5d-dst-override",
				Value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"L5d-Dst-Override": "%%CONTOUR_SERVICE_NAME%%.myns.svc.cluster.local:%%CONTOUR_SERVICE_PORT%%",
			},
		},
	}, {
		name: "default headers are combined with given headers and escaped",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"k-baz":           "%DOWNSTREAM_LOCAL_ADDRESS%", // This gets canonicalized
				"Lot-Of-Percents": "%%%%%",
			},
		},
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo":           "100%%",
				"K-Baz":           "%DOWNSTREAM_LOCAL_ADDRESS%",
				"Lot-Of-Percents": "%%%%%%%%%%",
			},
		},
	}, {
		name: "default headers do not replace given headers",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "50%",
			},
		},
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "100%%",
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotErr := headersPolicyService(test.dhp, test.in, test.dyn)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantErr, gotErr)
		})
	}
}
