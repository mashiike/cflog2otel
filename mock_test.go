package cflog2otel_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
)

type mockControler struct {
	tb      testing.TB
	objects []any
}

func newMockControler(tb testing.TB) *mockControler {
	return &mockControler{
		tb: tb,
	}
}

func (m *mockControler) Finish() {
	mock.AssertExpectationsForObjects(m.tb, m.objects...)
}

type mockDownloadAPIClient struct {
	mock.Mock
	tb testing.TB
}

func newMockDownloadAPIClient(ctrl *mockControler) *mockDownloadAPIClient {
	m := &mockDownloadAPIClient{
		tb: ctrl.tb,
	}
	ctrl.objects = append(ctrl.objects, m)
	return m
}

var _ manager.DownloadAPIClient = (*mockDownloadAPIClient)(nil)

func (m *mockDownloadAPIClient) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	ret := m.Called(ctx, input)
	output := ret.Get(0)
	if output == nil {
		return nil, ret.Error(1)
	}
	if o, ok := output.(*s3.GetObjectOutput); ok {
		return o, ret.Error(1)
	}
	m.tb.Errorf("unexpected type %T", output)
	return nil, ret.Error(1)
}
