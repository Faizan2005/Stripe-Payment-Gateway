package routes

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
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
