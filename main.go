package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	discordTokenEnv   = "DISCORD_TOKEN"
	mongoURIEnv       = "MONGO_URI"
	webhooksDatabase  = "webhooksDB"
	webhooksCollection = "webhooks"
)

var mongoClient *mongo.Client

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
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	discordToken := os.Getenv(discordTokenEnv)
	mongoURI := os.Getenv(mongoURIEnv)

	if discordToken == "" || mongoURI == "" {
		log.Fatalf("Missing environment variables: %s or %s", discordTokenEnv, mongoURIEnv)
	}

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	mongoClient = client
	defer mongoClient.Disconnect(context.TODO())

	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	dg.AddHandler(onReady)
	dg.AddHandler(onInteractionCreate)

	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening Discord session: %v", err)
	}
	defer dg.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Shutting down bot.")
}

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Bot is ready! Logged in as %s", event.User.Username)

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "create-webhook",
			Description: "Create a webhook in this channel.",
		},
		{
			Name:        "list-webhooks",
			Description: "List all webhooks in this server.",
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Fatalf("Cannot create '%s' command: %v", cmd.Name, err)
		}
	}
}

func onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch i.ApplicationCommandData().Name {
	case "create-webhook":
		createWebhook(s, i)
	case "list-webhooks":
		listWebhooks(s, i)
	}
}

func createWebhook(s *discordgo.Session, i *discordgo.InteractionCreate) {
	webhookName := "DefaultWebhookName"

	webhook, err := s.WebhookCreate(i.ChannelID, webhookName, "")
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to create webhook: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	webhookData := WebhookData{
		GuildID:      i.GuildID,
		ChannelID:    i.ChannelID,
		WebhookID:    webhook.ID,
		WebhookName:  webhook.Name,
		WebhookToken: webhook.Token,
	}

	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	_, err = collection.InsertOne(context.TODO(), webhookData)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to save webhook to database: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Webhook Created",
		Description: fmt.Sprintf("✅ Webhook created successfully: **%s**", webhook.Name),
		Color:       0x00FF00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Webhook Link",
				Value:  fmt.Sprintf("[Link](https://discord.com/api/webhooks/%s/%s)", webhook.ID, webhook.Token),
				Inline: false,
			},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func listWebhooks(s *discordgo.Session, i *discordgo.InteractionCreate) {
	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	cursor, err := collection.Find(context.TODO(), bson.M{"guild_id": i.GuildID})
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to retrieve webhooks: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	defer cursor.Close(context.TODO())

	var webhooks []WebhookData
	if err = cursor.All(context.TODO(), &webhooks); err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to parse webhooks: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if len(webhooks) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🔍 No webhooks found for this server.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Webhooks List",
		Color: 0x00FF00,
	}

	for _, webhook := range webhooks {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   webhook.WebhookName,
			Value:  fmt.Sprintf("[Link](https://discord.com/api/webhooks/%s/%s)", webhook.WebhookID, webhook.WebhookToken),
			Inline: false,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}
