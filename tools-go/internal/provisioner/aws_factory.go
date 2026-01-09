// Where: tools-go/internal/provisioner/aws_factory.go
// What: AWS client factory for DynamoDB/S3 provisioning.
// Why: Encapsulate SDK configuration for local endpoints.
package provisioner

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const defaultAWSRegion = "ap-northeast-1"

type ClientFactory interface {
	DynamoDB(ctx context.Context, endpoint string) (DynamoDBAPI, error)
	S3(ctx context.Context, endpoint string) (S3API, error)
}

type awsClientFactory struct{}

func (awsClientFactory) DynamoDB(ctx context.Context, endpoint string) (DynamoDBAPI, error) {
	cfg, err := loadAWSConfig(ctx, dynamodb.ServiceID, endpoint, dynamoAccessKey(), dynamoSecretKey())
	if err != nil {
		return nil, err
	}
	return awsDynamoClient{client: dynamodb.NewFromConfig(cfg)}, nil
}

func (awsClientFactory) S3(ctx context.Context, endpoint string) (S3API, error) {
	cfg, err := loadAWSConfig(ctx, s3.ServiceID, endpoint, s3AccessKey(), s3SecretKey())
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.UsePathStyle = true
	})
	return awsS3Client{client: client}, nil
}

func loadAWSConfig(
	ctx context.Context,
	serviceID string,
	endpoint string,
	accessKey string,
	secretKey string,
) (aws.Config, error) {
	if endpoint == "" {
		return aws.Config{}, fmt.Errorf("endpoint is required")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = defaultAWSRegion
	}

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, _ ...any) (aws.Endpoint, error) {
			if service != serviceID {
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
			}, nil
		},
	)

	creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(creds),
		config.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		return aws.Config{}, err
	}
	return cfg, nil
}

func dynamoAccessKey() string {
	if value := os.Getenv("DYNAMODB_ACCESS_KEY"); value != "" {
		return value
	}
	return "dummy"
}

func dynamoSecretKey() string {
	if value := os.Getenv("DYNAMODB_SECRET_KEY"); value != "" {
		return value
	}
	return "dummy"
}

func s3AccessKey() string {
	if value := os.Getenv("RUSTFS_ACCESS_KEY"); value != "" {
		return value
	}
	return "rustfsadmin"
}

func s3SecretKey() string {
	if value := os.Getenv("RUSTFS_SECRET_KEY"); value != "" {
		return value
	}
	return "rustfsadmin"
}
