package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"album-store-v1/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type fakeSQSClient struct {
	sendInput    *sqs.SendMessageInput
	sendErr      error
	receiveInput *sqs.ReceiveMessageInput
	receiveOut   *sqs.ReceiveMessageOutput
	receiveErr   error
	deleteInput  *sqs.DeleteMessageInput
	deleteErr    error
}

func (f *fakeSQSClient) SendMessage(_ context.Context, params *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendInput = params
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &sqs.SendMessageOutput{}, nil
}

func (f *fakeSQSClient) ReceiveMessage(_ context.Context, params *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	f.receiveInput = params
	if f.receiveErr != nil {
		return nil, f.receiveErr
	}
	if f.receiveOut != nil {
		return f.receiveOut, nil
	}
	return &sqs.ReceiveMessageOutput{}, nil
}

func (f *fakeSQSClient) DeleteMessage(_ context.Context, params *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.deleteInput = params
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &sqs.DeleteMessageOutput{}, nil
}

func TestPublishPhotoJobSendsExpectedMessage(t *testing.T) {
	client := &fakeSQSClient{}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	message := models.PhotoJobMessage{PhotoID: "photo-1"}
	if err := publisher.PublishPhotoJob(context.Background(), message); err != nil {
		t.Fatalf("publish photo job: %v", err)
	}

	if client.sendInput == nil {
		t.Fatal("expected send message to be called")
	}
	if aws.ToString(client.sendInput.QueueUrl) != "https://sqs.us-west-2.amazonaws.com/123/photos" {
		t.Fatalf("unexpected queue URL %q", aws.ToString(client.sendInput.QueueUrl))
	}

	var got models.PhotoJobMessage
	if err := json.Unmarshal([]byte(aws.ToString(client.sendInput.MessageBody)), &got); err != nil {
		t.Fatalf("unmarshal message body: %v", err)
	}
	if got != message {
		t.Fatalf("expected message %+v, got %+v", message, got)
	}
}

func TestPublishPhotoJobPropagatesQueueErrors(t *testing.T) {
	client := &fakeSQSClient{sendErr: errors.New("queue down")}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	if err := publisher.PublishPhotoJob(context.Background(), models.PhotoJobMessage{PhotoID: "photo-1"}); err == nil {
		t.Fatal("expected publish error")
	}
}

func TestReceivePhotoJobsDecodesQueueMessages(t *testing.T) {
	body, _ := json.Marshal(models.PhotoJobMessage{PhotoID: "photo-1"})
	client := &fakeSQSClient{
		receiveOut: &sqs.ReceiveMessageOutput{
			Messages: []types.Message{
				{
					Body:          aws.String(string(body)),
					ReceiptHandle: aws.String("receipt-1"),
				},
			},
		},
	}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	jobs, err := publisher.ReceivePhotoJobs(context.Background(), 5, 10)
	if err != nil {
		t.Fatalf("receive photo jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Message.PhotoID != "photo-1" || jobs[0].ReceiptHandle != "receipt-1" {
		t.Fatalf("unexpected jobs %+v", jobs)
	}
	if client.receiveInput == nil || client.receiveInput.MaxNumberOfMessages != 5 || client.receiveInput.WaitTimeSeconds != 10 {
		t.Fatalf("unexpected receive input %+v", client.receiveInput)
	}
}

func TestReceivePhotoJobsPropagatesDecodeErrors(t *testing.T) {
	client := &fakeSQSClient{
		receiveOut: &sqs.ReceiveMessageOutput{
			Messages: []types.Message{
				{
					Body:          aws.String("{"),
					ReceiptHandle: aws.String("receipt-1"),
				},
			},
		},
	}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	if _, err := publisher.ReceivePhotoJobs(context.Background(), 1, 1); err == nil {
		t.Fatal("expected receive decode error")
	}
}

func TestDeleteMessageUsesReceiptHandle(t *testing.T) {
	client := &fakeSQSClient{}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	if err := publisher.DeleteMessage(context.Background(), "receipt-1"); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if client.deleteInput == nil {
		t.Fatal("expected delete message to be called")
	}
	if aws.ToString(client.deleteInput.ReceiptHandle) != "receipt-1" {
		t.Fatalf("unexpected receipt handle %q", aws.ToString(client.deleteInput.ReceiptHandle))
	}
}

func TestDeleteMessagePropagatesErrors(t *testing.T) {
	client := &fakeSQSClient{deleteErr: errors.New("delete failed")}
	publisher := NewSQSPublisherWithClient(client, "https://sqs.us-west-2.amazonaws.com/123/photos")

	if err := publisher.DeleteMessage(context.Background(), "receipt-1"); err == nil {
		t.Fatal("expected delete error")
	}
}
