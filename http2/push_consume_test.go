// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http2

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"golang.org/x/net/http2/hpack"
)

func TestPushPromiseHeadersToHTTPRequest(t *testing.T) {
	headers := http.Header{}
	headers.Add("X", "y")
	getUrl := func(path, authority, scheme string) *url.URL {
		reqUrl, err := url.ParseRequestURI(path)
		if err != nil {
			t.Error(err)
			return nil
		}
		reqUrl.Host = authority
		reqUrl.Scheme = scheme
		return reqUrl
	}

	tests := []struct {
		name        string
		headers     []hpack.HeaderField
		expectedReq *http.Request
		expectedErr error
	}{
		{
			"NoErrors_IncludeNonRequiredHeaders",
			[]hpack.HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
				{Name: "X", Value: "y"},
			},
			&http.Request{
				Method:     "GET",
				Proto:      "HTTP/2.0",
				ProtoMajor: 2,
				URL:        getUrl("/hello", "foo.org", "https"),
				Header:     headers,
			},
			nil,
		},
		{
			"NoErrors_OnlyRequiredHeaders",
			[]hpack.HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
			},
			&http.Request{
				Method:     "GET",
				Proto:      "HTTP/2.0",
				ProtoMajor: 2,
				URL:        getUrl("/hello", "foo.org", "https"),
			},
			nil,
		},
		{
			"Missing_Method",
			[]hpack.HeaderField{
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
			},
			nil,
			errMissingHeaderMethod,
		},
		{
			"Missing_Scheme",
			[]hpack.HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
			},
			nil,
			errMissingHeaderScheme,
		},
		{
			"Missing_Authority",
			[]hpack.HeaderField{
				{Name: ":scheme", Value: "https"},
				{Name: ":method", Value: "GET"},
				{Name: ":path", Value: "/hello"},
			},
			nil,
			errMissingHeaderAuthority,
		},
		{
			"Missing_Path",
			[]hpack.HeaderField{
				{Name: ":scheme", Value: "https"},
				{Name: ":method", Value: "GET"},
				{Name: ":authority", Value: "foo.org"},
			},
			nil,
			errMissingHeaderPath,
		},
		{
			"Invalid_Method",
			[]hpack.HeaderField{
				{Name: ":method", Value: "POST"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
			},
			nil,
			errInvalidMethod,
		},
		{
			"Invalid_Scheme",
			[]hpack.HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "ftp"},
				{Name: ":authority", Value: "foo.org"},
				{Name: ":path", Value: "/hello"},
			},
			nil,
			errInvalidScheme,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpp := &MetaPushPromiseFrame{nil, tt.headers, false}
			req, err := pushedRequestToHTTPRequest(mpp)
			if err != tt.expectedErr {
				t.Fatalf("expected error %q but got error %q", tt.expectedErr, err)
			}
			if !reflect.DeepEqual(req, tt.expectedReq) {
				t.Fatalf("expected %v, but got %v", tt.expectedReq, req)
			}
		})
	}
}

type testPushHandlerRecordHandled struct {
	messageDone    bool
	requestHandled bool
}

func (ph *testPushHandlerRecordHandled) HandlePush(r *PushedRequest) {
	ph.requestHandled = true
	if ph.messageDone {
		r.pushedStream.done <- struct{}{}
	}
}

func TestHandlePushNoActionCancel(t *testing.T) {
	tests := []struct {
		name                 string
		returnBeforeComplete bool
		expectCancel         bool
	}{
		{
			"ReturnBeforeComplete",
			true,
			true,
		},
		{
			"ReturnAfterComplete",
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newServerTester(t, nil)
			defer st.Close()
			tr := &Transport{TLSClientConfig: tlsConfigInsecure}
			defer tr.CloseIdleConnections()
			cc, err := tr.dialClientConn(st.ts.Listener.Addr().String(), false)
			if err != nil {
				t.Fatal(err)
			}

			cs := cc.newStreamWithId(2, false)
			pr := &PushedRequest{pushedStream: cs}
			ph := &testPushHandlerRecordHandled{messageDone: !tt.returnBeforeComplete}
			handlePushEarlyReturnCancel(ph, pr)
			if cs.didReset && !tt.expectCancel {
				t.Error("expected pushed stream to be cancelled but it was not")
			} else if !cs.didReset && tt.expectCancel {
				t.Error("expected pushed stream to not be cancelled but it was")
			}
		})
	}
}
