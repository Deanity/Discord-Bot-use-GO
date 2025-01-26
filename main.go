package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/joho/godotenv"
)

// Environment variables
const (
	discordTokenEnv   = "DISCORD_TOKEN"
	mongoURIEnv       = "MONGO_URI"
	webhooksDatabase  = "webhooksDB"
	webhooksCollection = "webhooks"
)

// MongoDB Client
var mongoClient *mongo.Client

// Struct for webhook data
type WebhookData struct {
	GuildID      string `bson:"guild_id"`
	GuildName    string `bson:"guild_name"`
	ChannelID    string `bson:"channel_id"`
	ChannelName  string `bson:"channel_name"`
	WebhookID    string `bson:"webhook_id"`
	WebhookName  string `bson:"webhook_name"`
	WebhookToken string `bson:"webhook_token"`
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Load environment variables
	discordToken := os.Getenv(discordTokenEnv)
	mongoURI := os.Getenv(mongoURIEnv)

	if discordToken == "" || mongoURI == "" {
		log.Fatalf("Missing environment variables: %s or %s", discordTokenEnv, mongoURIEnv)
	}

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	mongoClient = client // Set the mongoClient variable

	// Ensure the MongoDB client disconnects when the program exits
	defer mongoClient.Disconnect(context.TODO())

	// Create a new Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Add event handlers
	dg.AddHandler(onReady)
	dg.AddHandler(onMessageCreate)

	// Open the websocket and begin listening
	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening Discord session: %v", err)
	}
	defer dg.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")
	// Wait for CTRL+C or other interrupt signals to exit
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Shutting down bot.")
}

// Event handler: Bot ready
func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Bot is ready! Logged in as %s", event.User.Username)
}

// Event handler: Message create
func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	// Handle commands
	if strings.HasPrefix(m.Content, "!create-webhook") {
		createWebhook(s, m)
	} else if strings.HasPrefix(m.Content, "!list-webhooks") {
		listWebhooks(s, m)
	}
}

// Create webhook command
func createWebhook(s *discordgo.Session, m *discordgo.MessageCreate) {
	args := strings.Split(m.Content, " ")
	webhookName := "BooKece"
	if len(args) > 1 {
		webhookName = args[1]
	}

	webhook, err := s.WebhookCreate(m.ChannelID, webhookName, "")
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Failed to create webhook: %v", err))
		return
	}

	webhookData := WebhookData{
		GuildID:      m.GuildID,
		GuildName:    m.GuildID, // Replace with GuildName if needed
		ChannelID:    m.ChannelID,
		ChannelName:  m.ChannelID, // Replace with ChannelName if needed
		WebhookID:    webhook.ID,
		WebhookName:  webhook.Name,
		WebhookToken: webhook.Token,
	}

	// Access MongoDB collection
	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	_, err = collection.InsertOne(context.TODO(), webhookData)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Failed to save webhook to database: %v", err))
		return
	}

	// Send success message with embed including the link
	embed := &discordgo.MessageEmbed{
		Title:       "Webhook Created",
		Description: fmt.Sprintf("✅ Webhook created successfully: **%s**", webhook.Name),
		Color:       0x00FF00, // Green color for success
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Webhook Link",
				Value:  fmt.Sprintf("https://discord.com/api/webhooks/%s/%s", webhook.ID, webhook.Token),
				Inline: false,
			},
		},
	}
 
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

// List webhooks command
func listWebhooks(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Access MongoDB collection
	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	cursor, err := collection.Find(context.TODO(), bson.M{"guild_id": m.GuildID})
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Failed to retrieve webhooks: %v", err))
		return
	}
	defer cursor.Close(context.TODO())

	var webhooks []WebhookData
	if err = cursor.All(context.TODO(), &webhooks); err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Failed to parse webhooks: %v", err))
		return
	}

	if len(webhooks) == 0 {
		s.ChannelMessageSend(m.ChannelID, "🔍 No webhooks found for this server.")
		return
	}

	// Create an embed message
	embed := &discordgo.MessageEmbed{
		Title: "Webhooks List",
		Color: 0x00FF00, // Green color for success
	}

	for _, webhook := range webhooks {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   webhook.WebhookName,
			Value:  fmt.Sprintf("[Link](https://discord.com/api/webhooks/%s/%s)", webhook.WebhookID, webhook.WebhookToken),
			Inline: false,
		})
	}

	// Send the embed message
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
