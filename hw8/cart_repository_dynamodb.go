package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var dynamoCartIDSeed = time.Now().UnixNano()

type DynamoDBCartRepository struct {
	client    *dynamodb.Client
	tableName string
}

type dynamoCartRecord struct {
	CartID     int64            `dynamodbav:"cart_id"`
	CustomerID int32            `dynamodbav:"customer_id"`
	Status     string           `dynamodbav:"status"`
	CreatedAt  string           `dynamodbav:"created_at"`
	UpdatedAt  string           `dynamodbav:"updated_at"`
	Items      map[string]int32 `dynamodbav:"items"`
}

func NewDynamoDBCartRepository(cfg Config) (*DynamoDBCartRepository, error) {
	awsConfig, err := awscfg.LoadDefaultConfig(
		context.Background(),
		awscfg.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &DynamoDBCartRepository{
		client:    dynamodb.NewFromConfig(awsConfig),
		tableName: cfg.DynamoDBTableName,
	}, nil
}

func (r *DynamoDBCartRepository) CreateCart(ctx context.Context, customerID int32) (*ShoppingCart, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := dynamoCartRecord{
		CartID:     nextDynamoCartID(),
		CustomerID: customerID,
		Status:     string(CartStatusActive),
		CreatedAt:  now,
		UpdatedAt:  now,
		Items:      map[string]int32{},
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return nil, fmt.Errorf("marshal dynamodb cart: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(cart_id)"),
	})
	if err != nil {
		return nil, fmt.Errorf("put dynamodb cart: %w", err)
	}

	return dynamoRecordToCart(record)
}

func (r *DynamoDBCartRepository) GetCart(ctx context.Context, cartID int64) (*ShoppingCart, error) {
	record, err := r.getCartRecord(ctx, cartID, false)
	if err != nil {
		return nil, err
	}

	return dynamoRecordToCart(*record)
}

func (r *DynamoDBCartRepository) AddItem(ctx context.Context, cartID int64, productID int32, quantity int32) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	productKey := strconv.Itoa(int(productID))

	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: strconv.FormatInt(cartID, 10)},
		},
		ConditionExpression: aws.String("attribute_exists(cart_id) AND #status = :active"),
		UpdateExpression: aws.String(
			"SET updated_at = :updated_at, #items.#product_id = if_not_exists(#items.#product_id, :zero) + :inc",
		),
		ExpressionAttributeNames: map[string]string{
			"#status":     "status",
			"#items":      "items",
			"#product_id": productKey,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active":     &types.AttributeValueMemberS{Value: string(CartStatusActive)},
			":updated_at": &types.AttributeValueMemberS{Value: now},
			":zero":       &types.AttributeValueMemberN{Value: "0"},
			":inc":        &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(quantity), 10)},
		},
	})
	if err == nil {
		return nil
	}

	var conditionalErr *types.ConditionalCheckFailedException
	if errors.As(err, &conditionalErr) {
		record, lookupErr := r.getCartRecord(ctx, cartID, true)
		if lookupErr != nil {
			if errors.Is(lookupErr, ErrNotFound) {
				return ErrNotFound
			}
			return lookupErr
		}

		if ShoppingCartStatus(record.Status) != CartStatusActive {
			return ErrCartClosed
		}
	}

	return fmt.Errorf("update dynamodb cart item: %w", err)
}

func (r *DynamoDBCartRepository) Close() error {
	return nil
}

func (r *DynamoDBCartRepository) getCartRecord(ctx context.Context, cartID int64, consistent bool) (*dynamoCartRecord, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: strconv.FormatInt(cartID, 10)},
		},
	}
	if consistent {
		input.ConsistentRead = aws.Bool(true)
	}

	output, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get dynamodb cart: %w", err)
	}
	if len(output.Item) == 0 {
		return nil, ErrNotFound
	}

	var record dynamoCartRecord
	if err := attributevalue.UnmarshalMap(output.Item, &record); err != nil {
		return nil, fmt.Errorf("unmarshal dynamodb cart: %w", err)
	}

	return &record, nil
}

func dynamoRecordToCart(record dynamoCartRecord) (*ShoppingCart, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	items := make([]ShoppingCartItem, 0, len(record.Items))
	for productID, quantity := range record.Items {
		parsedProductID, err := strconv.Atoi(productID)
		if err != nil {
			return nil, fmt.Errorf("parse product id %q: %w", productID, err)
		}

		items = append(items, ShoppingCartItem{
			ShoppingCartID: record.CartID,
			ProductID:      int32(parsedProductID),
			Quantity:       quantity,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ProductID < items[j].ProductID
	})

	return &ShoppingCart{
		ShoppingCartID: record.CartID,
		CustomerID:     record.CustomerID,
		Status:         ShoppingCartStatus(record.Status),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		Items:          items,
	}, nil
}

func nextDynamoCartID() int64 {
	return atomic.AddInt64(&dynamoCartIDSeed, 1)
}
