package admission

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	zv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type testAdmitter struct {
	// validate validates & plugs parameters for Admit
	validate func(response *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error)
}

func (t testAdmitter) Admit(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
	return t.validate(req)
}

func (t testAdmitter) AdmitAll(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
	return admissionv1.AdmissionResponse{
		Allowed: true,
	}, nil
}

func NewTestAdmitter() *testAdmitter {
	tadm := &testAdmitter{}
	tadm.validate = tadm.AdmitAll
	return tadm
}

func TestNonPostRequestsBad(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()
	h := Handler(tadm)
	h(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnsupportedContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()
	h := Handler(tadm)
	h(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRequestDecoding(t *testing.T) {
	expectedRg := zv1.RouteGroup{
		ObjectMeta: v1.ObjectMeta{
			Name:      "r1",
			Namespace: "n1",
		},
	}

	rb, err := json.Marshal(expectedRg)
	assert.NoError(t, err)

	review := &admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Name:      "r1",
			Namespace: "n1",
			// TODO: why doesnt runtime.RawExtension{Object: rg} work here?
			Object: runtime.RawExtension{Raw: rb},
		},
	}

	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)
	err = enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()

	tadm.validate = func(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
		// TODO: add a differ here so the message is more readable
		assert.Equal(t, review.Request, req, "AdmissionReview.Request is not as expected")

		// decode a RouteGroup from req.Object.Raw and validate it
		var rg definitions.RouteGroupItem
		err := json.Unmarshal(req.Object.Raw, &rg)
		assert.NoError(t, err)

		assert.Equal(t, expectedRg.Name, rg.Metadata.Name)
		assert.Equal(t, expectedRg.Namespace, rg.Metadata.Namespace)

		return admissionv1.AdmissionResponse{
			Allowed: true,
		}, nil
	}

	h := Handler(tadm)
	h(w, req)

}

func TestResponseEncoding(t *testing.T) {
	review := &admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Name:      "r1",
			Namespace: "n1",
		},
	}

	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)
	err := enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()

	h := Handler(tadm)
	h(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	admresp := &admissionv1.AdmissionResponse{}
	rb, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err, "could not read response")
	//  that verifies the AdmissionReview{} Received and sends back a test
	//  response
	err = json.Unmarshal(rb, &admresp)
	assert.NoError(t, err, "could not parse admission response")
}
