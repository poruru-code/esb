// Where: cli/internal/provisioner/provisioner_test.go
// What: Tests for provisioner helpers and provisioning flows.
// Why: Ensure DynamoDB/S3 provisioning behaves consistently with expectations.
package provisioner

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
)

type fakePortResolver struct {
	port  int
	err   error
	calls int
}

func (f *fakePortResolver) Resolve(_ context.Context, _ PortRequest) (int, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	return f.port, nil
}

func TestResolvePortUsesEnv(t *testing.T) {
	t.Setenv("ESB_PORT_DATABASE", "8001")
	resolver := &fakePortResolver{port: 9999}

	port, ok := resolvePort(context.Background(), "ESB_PORT_DATABASE", 8000, PortRequest{}, resolver)
	if !ok {
		t.Fatalf("expected port to resolve")
	}
	if port != 8001 {
		t.Fatalf("unexpected port: %d", port)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected resolver not called")
	}
}

func TestResolvePortUsesResolverWhenZero(t *testing.T) {
	t.Setenv("ESB_PORT_S3", "0")
	resolver := &fakePortResolver{port: 9002}

	port, ok := resolvePort(context.Background(), "ESB_PORT_S3", 9000, PortRequest{}, resolver)
	if !ok {
		t.Fatalf("expected port to resolve")
	}
	if port != 9002 {
		t.Fatalf("unexpected port: %d", port)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected resolver called once")
	}
}

func TestResolvePortFallsBackToDefaultWhenUnset(t *testing.T) {
	t.Setenv("ESB_PORT_S3", "")
	resolver := &fakePortResolver{err: errors.New("not found")}

	port, ok := resolvePort(context.Background(), "ESB_PORT_S3", 9000, PortRequest{}, resolver)
	if !ok {
		t.Fatalf("expected port to resolve")
	}
	if port != 9000 {
		t.Fatalf("unexpected port: %d", port)
	}
}

type fakeDynamo struct {
	existing []string
	created  []DynamoCreateInput
}

func (f *fakeDynamo) ListTables(_ context.Context) ([]string, error) {
	return f.existing, nil
}

func (f *fakeDynamo) CreateTable(_ context.Context, input DynamoCreateInput) error {
	f.created = append(f.created, input)
	return nil
}

type fakeS3 struct {
	existing      []string
	created       []string
	lifecycleConf map[string]interface{}
}

func (f *fakeS3) ListBuckets(_ context.Context) ([]string, error) {
	return f.existing, nil
}

func (f *fakeS3) CreateBucket(_ context.Context, name string) error {
	f.created = append(f.created, name)
	return nil
}

func (f *fakeS3) PutBucketLifecycleConfiguration(_ context.Context, name string, config *schema.AWSS3BucketLifecycleConfiguration) error {
	if config == nil {
		return errors.New("invalid lifecycle configuration format")
	}
	if f.lifecycleConf == nil {
		f.lifecycleConf = make(map[string]interface{})
	}
	f.lifecycleConf[name] = config
	return nil
}

func TestProvisionDynamoCreatesWhenMissing(t *testing.T) {
	client := &fakeDynamo{existing: []string{}}
	tables := []generator.DynamoDBSpec{
		{
			TableName: "test-table",
			KeySchema: []schema.AWSDynamoDBTableKeySchema{
				{AttributeName: "id", KeyType: "HASH"},
			},
			AttributeDefinitions: []schema.AWSDynamoDBTableAttributeDefinition{
				{AttributeName: "id", AttributeType: "S"},
			},
			BillingMode: "PAY_PER_REQUEST",
		},
	}

	var out bytes.Buffer
	provisionDynamo(context.Background(), client, tables, &out)

	if len(client.created) != 1 {
		t.Fatalf("expected table created once, got %d", len(client.created))
	}
	if client.created[0].TableName != "test-table" {
		t.Fatalf("unexpected table name: %s", client.created[0].TableName)
	}
}

func TestProvisionDynamoSkipsExisting(t *testing.T) {
	client := &fakeDynamo{existing: []string{"test-table"}}
	tables := []generator.DynamoDBSpec{
		{TableName: "test-table"},
	}

	var out bytes.Buffer
	provisionDynamo(context.Background(), client, tables, &out)

	if len(client.created) != 0 {
		t.Fatalf("expected no table creation")
	}
}

func TestProvisionS3CreatesWhenMissing(t *testing.T) {
	client := &fakeS3{existing: []string{}}
	buckets := []generator.S3Spec{
		{BucketName: "test-bucket"},
	}

	var out bytes.Buffer
	provisionS3(context.Background(), client, buckets, &out)

	if len(client.created) != 1 || client.created[0] != "test-bucket" {
		t.Fatalf("unexpected bucket creation: %v", client.created)
	}
}

func TestProvisionS3SkipsExisting(t *testing.T) {
	client := &fakeS3{existing: []string{"test-bucket"}}
	buckets := []generator.S3Spec{
		{BucketName: "test-bucket"},
	}

	var out bytes.Buffer
	provisionS3(context.Background(), client, buckets, &out)

	if len(client.created) != 0 {
		t.Fatalf("expected no bucket creation")
	}
}
