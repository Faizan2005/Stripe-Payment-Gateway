package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/paymentintent"
	"github.com/stripe/stripe-go/v78/refund"
	"github.com/stripe/stripe-go/v78/webhook"
)

var stripeKey = os.Getenv("STRIPE_SECRET_KEY")

type APIServer struct {
	listenAddr string
}

func NewAPIServer(listenAddr string) *APIServer {
	return &APIServer{listenAddr: listenAddr}
}

func (s *APIServer) Run() {
	app := fiber.New()

	api1 := app.Group("/payment")
	api2 := app.Group("/subscription")

	api1.Post("/intent", HandlePaymentRequest)
	api1.Post("/webhook", HandleStripeWebhook)
	api1.Post("/refund", HandlePaymentRefund)
	api1.Post("/cancel", HandlePaymentRequest)

	api2.Post("/create", HandlePaymentRequest)
	api2.Post("/cancel", HandlePaymentRequest)

	app.Get("/transactions", HandlePaymentRequest)

	if err := app.Listen(s.listenAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type Payment struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PaymentMethod string `json:"paymentMethod"`
}

func HandlePaymentRequest(c *fiber.Ctx) error {
	var p Payment
	if err := c.BodyParser(&p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	stripe.Key = stripeKey

	supportedPaymentMethods := map[string]bool{
		"card":          true,
		"upi":           true,
		"wallet":        true,
		"bank_transfer": true,
	}

	if !supportedPaymentMethods[p.PaymentMethod] {
		return c.Status(400).JSON(fiber.Map{"error": "Unsupported payment method"})
	}

	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(p.Amount),
		Currency:           stripe.String(p.Currency),
		PaymentMethodTypes: stripe.StringSlice([]string{p.PaymentMethod}),
	}

	result, err := paymentintent.New(params)
	if err != nil {
		log.Println("PaymentIntent error:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Payment failed"})
	}

	return c.JSON(fiber.Map{
		"message":        "Payment initiated",
		"payment_intent": result.ID,
		"client_secret":  result.ClientSecret,
		"status":         result.Status,
	})
}

func HandleStripeWebhook(c *fiber.Ctx) error {
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

	case "payment_intent.payment_failed":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			log.Println("Error parsing payment_intent.payment_failed:", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("PaymentIntent failed! ID: %s\n", paymentIntent.ID)

	case "payment_intent.processing":
		fmt.Println("Payment is being processed. Waiting for confirmation.")

	case "payment_intent.requires_action":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			log.Println("Error parsing payment_intent.requires_action:", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("User action required! Follow this URL: %s\n", paymentIntent.NextAction.RedirectToURL.URL)

	default:
		fmt.Printf("Unhandled event type: %s\n", event.Type)
	}

	return c.SendStatus(fiber.StatusOK)
}

func HandlePaymentRefund(c *fiber.Ctx) error {
	var request struct {
		PaymentIntentID string `json:"paymentIntentID"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if request.PaymentIntentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "PaymentIntent ID is required"})
	}

	stripe.Key = stripeKey
	params := &stripe.RefundParams{PaymentIntent: stripe.String(request.PaymentIntentID)}

	result, err := refund.New(params)
	if err != nil {
		log.Println("PaymentIntent error:", err)
		return c.Status(500).JSON(fiber.Map{"error": "Refund failed"})
	}

	return c.JSON(fiber.Map{
		"message":   "Refund initiated",
		"refund_id": result.ID,
		"status":    result.Status,
	})
}
