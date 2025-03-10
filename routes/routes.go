package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v75/webhook"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/paymentintent"
)

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

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(p.Amount),
		Currency: stripe.String(p.Currency),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card", "upi", "wallet", "bank_transfer",
		}),
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
	})
}

func HandleStripeWebhook(c *fiber.Ctx) error {
	stripeWebhookSecret := os.Getenv("STRIPE_SECRET_KEY")

	payload := c.Body()

	signature := c.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, signature, stripeWebhookSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Webhook signature verification failed: %v\n", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid signature"})
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing payment_intent.succeeded: %v\n", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("PaymentIntent was successful! ID: %s\n", paymentIntent.ID)

	case "payment_intent.payment_failed":
		var paymentIntent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing payment_intent.payment_failed: %v\n", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
		}
		fmt.Printf("PaymentIntent failed! ID: %s\n", paymentIntent.ID)

	default:
		fmt.Printf("Unhandled event type: %s\n", event.Type)
	}

	return c.SendStatus(fiber.StatusOK)
}
