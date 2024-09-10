package cflog2otel_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mashiike/cflog2otel"
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

type mockS3APIClient struct {
	mock.Mock
	tb testing.TB
}

func newMockS3APIClient(ctrl *mockControler) *mockS3APIClient {
	m := &mockS3APIClient{
		tb: ctrl.tb,
	}
	ctrl.objects = append(ctrl.objects, m)
	return m
}

var _ cflog2otel.S3APIClient = (*mockS3APIClient)(nil)

func (m *mockS3APIClient) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.tb.Helper()
	m.tb.Log("GetObject", "bucket", *input.Bucket, "key", *input.Key)
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

func (m *mockS3APIClient) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	m.tb.Helper()
	m.tb.Log("ListObjectsV2", "bucket", *input.Bucket)
	ret := m.Called(ctx, input)
	output := ret.Get(0)
	if output == nil {
		return nil, ret.Error(1)
	}
	if o, ok := output.(*s3.ListObjectsV2Output); ok {
		return o, ret.Error(1)
	}
	m.tb.Errorf("unexpected type %T", output)
	return nil, ret.Error(1)
}
