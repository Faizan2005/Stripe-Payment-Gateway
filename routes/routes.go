package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Faizan2005/payment-gateway-stripe/models"
	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/paymentintent"
	"github.com/stripe/stripe-go/v78/refund"
	"github.com/stripe/stripe-go/v78/subscription"
	"github.com/stripe/stripe-go/v78/webhook"
)

var stripeKey = os.Getenv("STRIPE_SECRET_KEY")

type APIServer struct {
	listenAddr string
	storage    models.Storage
}

func NewAPIServer(listenAddr string, storage models.Storage) *APIServer {
	return &APIServer{listenAddr: listenAddr,
		storage: storage}
}

func (s *APIServer) Run() {
	app := fiber.New()

	api1 := app.Group("/payment")
	api2 := app.Group("/subscription")

	api1.Post("/intent", s.HandlePaymentRequest)
	api1.Post("/webhook", s.HandleStripeWebhook)
	api1.Post("/refund", s.HandlePaymentRefund)
	api1.Post("/cancel", s.HandleCancelPayment)

	api2.Post("/create", s.HandleCreateSubscription)
	api2.Post("/cancel", s.HandleCancelSubscription)

	app.Get("/transactions", s.HandleGetTransactions)

	if err := app.Listen(s.listenAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func (s *APIServer) HandlePaymentRequest(c *fiber.Ctx) error {
	var p models.Payment
	if err := c.BodyParser(&p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Create or retrieve user
	_, userID, err := s.HandleCreateCustomer(p.Name, p.Email) // Call the customer creation function
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create or retrieve user"})
	}

	// Create PaymentIntent
	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(p.Amount),
		Currency:           stripe.String(p.Currency),
		PaymentMethodTypes: stripe.StringSlice([]string{p.PaymentMethod}),
	}

	result, err := paymentintent.New(params)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Payment failed"})
	}

	// Store payment in the database
	payID, err := s.storage.CreatePayment(userID, p.Name, p.Email, p.Amount, p.Currency, p.PaymentMethod, result.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to store payment"})
	}

	// Update payment status to pending
	err = s.storage.UpdatePaymentStatus(result.ID, "pending")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update payment status"})
	}

	err = s.storage.LogTransaction(p.UserID, "payment", p.Amount, p.Currency, &payID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to store transaction details"})
	}

	return c.JSON(fiber.Map{
		"message":        "Payment initiated",
		"payment_intent": result.ID,
		"client_secret":  result.ClientSecret,
	})
}

func (s *APIServer) HandleStripeWebhook(c *fiber.Ctx) error {
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhookSecret == "" {
		log.Fatal("Missing STRIPE_WEBHOOK_SECRET environment variable")
	}

	payload := c.Body()
	signature := c.Get("Stripe-Signature")

	if signature == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing signature"})
	}

	event, err := webhook.ConstructEvent(payload, signature, stripeWebhookSecret)
	if err != nil {
		log.Println("Webhook signature verification failed:", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			log.Println("Error parsing payment_intent.succeeded:", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("Payment successful: ID=%s, Amount=%d %s, Status=%s\n", paymentIntent.ID, paymentIntent.Amount, paymentIntent.Currency, paymentIntent.Status)

		// Update payment status in the database
		err = s.storage.UpdatePaymentStatus(paymentIntent.ID, "success")
		if err != nil {
			log.Println("Failed to update payment status:", err)
		}

	case "payment_intent.payment_failed":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			log.Println("Error parsing payment_intent.payment_failed:", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("PaymentIntent failed! ID: %s\n", paymentIntent.ID)

		// Update payment status in the database
		err = s.storage.UpdatePaymentStatus(paymentIntent.ID, "failed")
		if err != nil {
			log.Println("Failed to update payment status:", err)
		}

	default:
		fmt.Printf("Unhandled event type: %s\n", event.Type)
	}

	return c.SendStatus(fiber.StatusOK)
}

func (s *APIServer) HandlePaymentRefund(c *fiber.Ctx) error {
	var request struct {
		PaymentIntentID string `json:"paymentIntentID"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if request.PaymentIntentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "PaymentIntent ID is required"})
	}

	p, err := s.storage.GetPaymentDetails(request.PaymentIntentID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve payment details"})
	}

	stripe.Key = stripeKey

	paymentIntent, err := paymentintent.Get(request.PaymentIntentID, nil)
	if err != nil {
		log.Println("Error fetching PaymentIntent:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch payment details"})
	}

	if paymentIntent.Status == "succeeded" {
		params := &stripe.RefundParams{PaymentIntent: stripe.String(request.PaymentIntentID)}
		result, err := refund.New(params)
		if err != nil {
			log.Println("Refund error:", err)
			return c.Status(500).JSON(fiber.Map{"error": "Refund failed"})
		}

		refID, err := s.storage.CreateRefund(p.ID, p.Amount, string(result.Status), result.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to store refund"})
		}

		err = s.storage.LogTransaction(p.UserID, "refund", p.Amount, p.Currency, &refID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to store transaction details"})
		}

		// Update refund status to refunded
		err = s.storage.UpdateRefundStatus(result.ID, "refunded")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update refund status"})
		}

		return c.JSON(fiber.Map{
			"message":   "Refund initiated",
			"refund_id": result.ID,
			"status":    result.Status,
		})
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "Payment not successful, cannot refund"})
	}
}

func (s *APIServer) HandleCancelPayment(c *fiber.Ctx) error {
	var request struct {
		PaymentIntentID string `json:"paymentIntentID"`
	}

	p, err := s.storage.GetPaymentDetails(request.PaymentIntentID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve payment details"})
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	params := &stripe.PaymentIntentCancelParams{}
	result, err := paymentintent.Cancel(request.PaymentIntentID, params)
	if err != nil {
		log.Println("PaymentIntent error:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Cancellation failed"})
	}

	err = s.storage.CancelPayment(p.ID, p.UserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to remove payment details from database"})
	}

	err = s.storage.UpdatePaymentStatus(p.StripePaymentID, "canceled")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update payment status"})
	}

	return c.JSON(fiber.Map{
		"message":   "Cancellation initiated",
		"cancel_id": result.ID,
		"status":    result.Status,
	})
}

func (s *APIServer) HandleCreateSubscription(c *fiber.Ctx) error {
	var sub models.Subscription
	if err := c.BodyParser(&sub); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	stripe.Key = stripeKey

	usrID := string(sub.UserID)
	amount := string(sub.Amount)

	params := &stripe.SubscriptionParams{
		Customer: stripe.String(usrID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(amount), // Price ID must be dynamic
			},
		},
	}

	result, err := subscription.New(params)
	if err != nil {
		log.Println("Subscription creation error:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Subscription creation failed"})
	}

	err = s.storage.CreateSubscription(sub.UserID, sub.PaymentID, sub.Amount, sub.Currency, string(result.ID), string(result.Status))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to store subscription details"})
	}

	return c.JSON(fiber.Map{
		"message":         "Subscription created",
		"subscription_id": result.ID,
		"status":          result.Status,
	})
}

func (s *APIServer) HandleCancelSubscription(c *fiber.Ctx) error {
	var request struct {
		SubscriptionID string `json:"subscription_id"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	sub, err := s.storage.GetSubscriptionDetails(request.SubscriptionID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve subscription details"})
	}

	params := &stripe.SubscriptionCancelParams{}
	result, err := subscription.Cancel(request.SubscriptionID, params)
	if err != nil {
		log.Println("Subscription cancellation error:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Subscription cancellation failed"})
	}

	err = s.storage.CancelSubscription(sub.ID, sub.UserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to remove subscription details from database"})
	}

	err = s.storage.UpdateSubscriptionStatus(sub.StripeSubscriptionID, "canceled")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update subscription status"})
	}

	return c.JSON(fiber.Map{
		"message":   "Subscription cancelled",
		"cancel_id": result.ID,
		"status":    result.Status,
	})
}

func (s *APIServer) HandleCreateCustomer(name, email string) (string, uint, error) {
	stripeID, userID, err := s.storage.CheckCustomer(name, email)
	if err == nil {
		return stripeID, userID, nil
	}

	stripe.Key = stripeKey

	params := &stripe.CustomerParams{
		Name:  stripe.String(name),
		Email: stripe.String(email),
	}

	result, err := customer.New(params)
	if err != nil {
		log.Println("User creation error:", err)
		return "", 0, fmt.Errorf("user creation failed")
	}

	_, userID, err = s.storage.CreateCustomer(name, email, result.ID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create user in database")
	}

	return result.ID, userID, nil
}

func (s *APIServer) HandleGetTransactions(c *fiber.Ctx) error {
	var t models.Transaction
	Transactions, err := s.storage.GetUserTransactions(t.UserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(Transactions)
}
