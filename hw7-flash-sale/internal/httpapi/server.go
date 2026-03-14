package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hw7-flash-sale/internal/config"
	"hw7-flash-sale/internal/models"
	"hw7-flash-sale/internal/orders"
)

func NewRouter(orderService *orders.Service, cfg config.Config) *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":               "ok",
			"messaging_backend":    cfg.MessagingBackend,
			"payment_delay_secs":   int(cfg.PaymentDelay.Seconds()),
			"sync_payment_slots":   cfg.SyncPaymentSlots,
			"events_dir":           cfg.EventsDir,
			"sns_topic_arn_set":    cfg.SNSTopicARN != "",
			"sqs_queue_url_set":    cfg.SQSQueueURL != "",
			"implemented_features": []string{"health", "sync-orders", "async-order-acceptance", "file-or-aws-messaging-backend"},
		})
	})

	router.POST("/orders/sync", func(c *gin.Context) {
		req, ok := bindCreateOrderRequest(c)
		if !ok {
			return
		}

		order, err := orderService.ProcessSync(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "PAYMENT_FAILED",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, order)
	})

	router.POST("/orders/async", func(c *gin.Context) {
		req, ok := bindCreateOrderRequest(c)
		if !ok {
			return
		}

		order, err := orderService.AcceptAsync(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "PUBLISH_FAILED",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, order)
	})

	return router
}

func bindCreateOrderRequest(c *gin.Context) (models.CreateOrderRequest, bool) {
	var req models.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: err.Error(),
		})
		return models.CreateOrderRequest{}, false
	}

	return req, true
}
