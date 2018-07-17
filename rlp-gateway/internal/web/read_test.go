package web_test

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Read", func() {
	var (
		lp *stubLogsProvider
	)
	BeforeEach(func() {
		lp = newStubLogsProvider()
		lp._batchResponse = &loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{
				{
					SourceId: "source-id-a",
				},
				{
					SourceId: "source-id-b",
				},
			},
		}
	})

	It("reads from the logs provider and sends SSE to the client", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v2/read", nil)
		req = req.WithContext(ctx)

		h := web.ReadHandler(lp)

		go h.ServeHTTP(rec, req)

		// TODO: How to expect on these?
		// Expect(rec.Code).To(Equal(http.StatusOK))

		Eventually(lp.requests).Should(HaveLen(1))
		Expect(rec.HeaderMap.Get("Content-Type")).To(Equal("text/event-stream"))
		Expect(rec.HeaderMap.Get("Cache-Control")).To(Equal("no-cache"))
		Expect(rec.HeaderMap.Get("Connection")).To(Equal("keep-alive"))

		buf := bufio.NewReader(rec.Body)

		line, err := buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())
		Expect(string(line)).To(Equal(`data: {"batch":[{"sourceId":"source-id-a"},{"sourceId":"source-id-b"}]}` + "\n"))

		// Read 1 empty new lines
		_, err = buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())

		line, err = buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())
		Expect(string(line)).To(Equal(`data: {"batch":[{"sourceId":"source-id-a"},{"sourceId":"source-id-b"}]}` + "\n"))
	})

	It("closes the SSE stream if the envelope stream returns any error", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v2/read", nil)
		req = req.WithContext(ctx)

		lp._batchResponse = nil
		lp._errorResponse = errors.New("an error")

		h := web.ReadHandler(lp)

		go h.ServeHTTP(rec, req)

		Eventually(func() int { return rec.Code }).Should(Equal(http.StatusGone))
	})
})

type stubLogsProvider struct {
	mu             sync.Mutex
	_requests      []*loggregator_v2.EgressBatchRequest
	_batchResponse *loggregator_v2.EnvelopeBatch
	_errorResponse error
}

func newStubLogsProvider() *stubLogsProvider {
	return &stubLogsProvider{}
}

func (s *stubLogsProvider) Stream(ctx context.Context, req *loggregator_v2.EgressBatchRequest) web.Receiver {
	s._requests = append(s._requests, req)

	return func() (*loggregator_v2.EnvelopeBatch, error) {
		return s._batchResponse, s._errorResponse
	}
}

func (s *stubLogsProvider) requests() []*loggregator_v2.EgressBatchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._requests
}
