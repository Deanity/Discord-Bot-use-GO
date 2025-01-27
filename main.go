package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/joho/godotenv"

)

const (
	discordTokenEnv   = "DISCORD_TOKEN"
	mongoURIEnv       = "MONGO_URI"
	serversDatabase   = "serverStatsDB"
	serversCollection = "servers"
	webhooksDatabase  = "webhooksDB"
	webhooksCollection = "webhooks"
)

var mongoClient *mongo.Client

type ServerData struct {
	GuildID     string `bson:"guild_id"`
	GuildName   string `bson:"guild_name"`
	MemberCount int    `bson:"member_count"`
	JoinedAt    string `bson:"joined_at"`
}

type WebhookData struct {
	GuildID      string `bson:"guild_id"`
	GuildName    string `bson:"guild_name"`
	ChannelID    string `bson:"channel_id"`
	ChannelName  string `bson:"channel_name"`
	WebhookID    string `bson:"webhook_id"`
	WebhookName  string `bson:"webhook_name"`
	WebhookToken string `bson:"webhook_token"`
}

// sendEphemeralMessage mengirimkan pesan ephemeral ke user tertentu
func sendEphemeralMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral, // Menandai pesan sebagai ephemeral
		},
	})
	if err != nil {
		log.Printf("Gagal mengirim pesan ephemeral: %v", err)
	}
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
	defer func() {
		if err := mongoClient.Disconnect(context.TODO()); err != nil {
			log.Printf("Error disconnecting MongoDB: %v", err)
		}
	}()
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}
	dg.AddHandler(onReady)
	dg.AddHandler(onGuildCreate)
	dg.AddHandler(onGuildDelete)
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
			Description: "Create a new webhook in this channel",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Name of the webhook",
					Required:    false,
				},
			},
		},
		{
			Name:        "list-webhooks",
			Description: "List all webhooks in this server",
		},
		{
			Name:        "server-stats",
			Description: "Displays a list of servers where the bot is present.",
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Fatalf("Failed to register command %s: %v", cmd.Name, err)
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
	case "server-stats":
		listServerStats(s, i)
	default:
		log.Printf("Unhandled command: %s", i.ApplicationCommandData().Name)
	}
}

func onGuildCreate(_ *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	collection := mongoClient.Database(serversDatabase).Collection(serversCollection)
	serverData := ServerData{
		GuildID:     event.Guild.ID,
		GuildName:   event.Guild.Name,
		MemberCount: event.Guild.MemberCount,
		JoinedAt:    event.JoinedAt.String(),
	}
	_, err := collection.InsertOne(context.TODO(), serverData)
	if err != nil {
		log.Printf("Error saving server to database: %v", err)
		return
	}
	log.Printf("Bot joined server: %s (%s)", event.Guild.Name, event.Guild.ID)
}

func onGuildDelete(_ *discordgo.Session, event *discordgo.GuildDelete) {
	if event.Unavailable {
		return
	}

	collection := mongoClient.Database(serversDatabase).Collection(serversCollection)
	_, err := collection.DeleteOne(context.TODO(), bson.M{"guild_id": event.Guild.ID})
	if err != nil {
		log.Printf("Error removing server from database: %v", err)
		return
	}
	log.Printf("Bot left server: %s (%s)", event.Guild.Name, event.Guild.ID)
}

func createWebhook(s *discordgo.Session, i *discordgo.InteractionCreate) {
	webhookName := "BooKece"

	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		webhookName = options[0].StringValue()
	}

	webhook, err := s.WebhookCreate(i.ChannelID, webhookName, "")
	if err != nil {
		sendEphemeralMessage(s, i, fmt.Sprintf("❌ Failed to create webhook: %v", err))
		return
	}

	webhookData := WebhookData{
		GuildID:      i.GuildID,
		GuildName:    "Unknown Guild",
		ChannelID:    i.ChannelID,
		ChannelName:  "Unknown Channel",
		WebhookID:    webhook.ID,
		WebhookName:  webhook.Name,
		WebhookToken: webhook.Token,
	}

	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	_, err = collection.InsertOne(context.TODO(), webhookData)
	if err != nil {
		sendEphemeralMessage(s, i, fmt.Sprintf("❌ Failed to save webhook to database: %v", err))
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Webhook Created",
		Description: fmt.Sprintf("✅ Webhook created successfully: **%s**", webhook.Name),
		Color:       0x00FF00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Webhook Link",
				Value:  fmt.Sprintf("https://discord.com/api/webhooks/%s/%s", webhook.ID, webhook.Token),
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
	log.Println("Webhook created successfully.")
}


func listWebhooks(s *discordgo.Session, i *discordgo.InteractionCreate) {
	collection := mongoClient.Database(webhooksDatabase).Collection(webhooksCollection)
	cursor, err := collection.Find(context.TODO(), bson.M{"guild_id": i.GuildID})
	if err != nil {
		sendEphemeralMessage(s, i, fmt.Sprintf("❌ Failed to retrieve webhooks: %v", err))
		return
	}
	defer cursor.Close(context.TODO())

	var webhooks []WebhookData
	if err = cursor.All(context.TODO(), &webhooks); err != nil {
		sendEphemeralMessage(s, i, fmt.Sprintf("❌ Failed to parse webhooks: %v", err))
		return
	}

	if len(webhooks) == 0 {
		sendEphemeralMessage(s, i, "🔍 No webhooks found for this server.")
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
		},
	})
	log.Println("Listed webhooks.")
}


func listServerStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	collection := mongoClient.Database(serversDatabase).Collection(serversCollection)
	cursor, err := collection.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Printf("Failed to retrieve server stats: %v", err)
		return
	}
	defer cursor.Close(context.TODO())

	var servers []ServerData
	if err = cursor.All(context.TODO(), &servers); err != nil {
		log.Printf("Failed to parse server stats: %v", err)
		return
	}

	if len(servers) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No servers found in the database.",
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Server Stats",
		Color: 0x00FF00,
	}

	for _, server := range servers {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   server.GuildName,
			Value:  fmt.Sprintf("ID: %s\nMembers: %d\nJoined: %s", server.GuildID, server.MemberCount, server.JoinedAt),
			Inline: false,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	log.Println("Listed Server")
}
