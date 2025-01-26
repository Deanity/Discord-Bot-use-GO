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
	discordTokenEnv    = "DISCORD_TOKEN"
	mongoURIEnv        = "MONGO_URI"
	webhooksDatabase   = "webhooksDB"
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
	dg.AddHandler(onMessageCreate)

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
	}
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == "tutorwebhook" {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0x3498DB,
			Image: &discordgo.MessageEmbedImage{
				URL: "https://cdn.discordapp.com/attachments/1282966917430120516/1333097030666682389/Teks_paragraf_Anda_6.png?ex=6797a6db&is=6796555b&hm=825c7475ff579ac606a9a6309d31d8250c34f5b206d9225336b9877242d8c321&",
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Tutorial Webhook",
					Value:  "Ikutin garis kuning dan anak panah \n Invite Bot <a:live:1247888274161143878> <a:panah:1333099217404821597> <@1331944037908742205>",
					Inline: false,
				},
			},
		})
	}
}

func createWebhook(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Default nama webhook
	webhookName := "BooKece"

	// Mengambil input nama webhook dari opsi (jika ada)
	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		webhookName = options[0].StringValue()
	}

	// Membuat webhook baru
	webhook, err := s.WebhookCreate(i.ChannelID, webhookName, "")
	if err != nil {
		sendEphemeralMessage(s, i, fmt.Sprintf("❌ Failed to create webhook: %v", err))
		return
	}

	// Menyimpan data webhook ke MongoDB
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

	// Mengirimkan respon sukses dengan detail webhook
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
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendEphemeralMessage(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
