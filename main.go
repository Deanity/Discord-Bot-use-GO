package main

import (
	"context"
	// "encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	// "time"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// MongoDB Atlas connection string
	mongoURI = "mongodb+srv://BooKece:Boolua@cluster0.h9luy.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"

	// Database and collection names
	dbName         = "discord_bot"
	webhooksCol    = "webhooks"
	serversCol     = "servers"
	clientInstance *mongo.Client
)

func main() {
	// Load environment variables
	token := os.Getenv("MTMzMTk0NDAzNzkwODc0MjIwNQ.G2Oqlf.D4jZLzmh0SdMAf5Jm_r1-EX2wxEd8_oJ7AZTgA")
	if token == "" {
		log.Fatal("❌ Token bot tidak ditemukan dalam environment variables.")
	}

	// Initialize MongoDB client
	var err error
	clientInstance, err = mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("❌ Gagal menghubungkan ke MongoDB: %v", err)
	}
	defer clientInstance.Disconnect(context.TODO())

	// Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("❌ Gagal membuat session Discord: %v", err)
	}

	// Add event handlers
	dg.AddHandler(onReady)
	dg.AddHandler(onGuildCreate)
	dg.AddHandler(onGuildDelete)
	dg.AddHandler(onInteraction)

	// Open WebSocket
	err = dg.Open()
	if err != nil {
		log.Fatalf("❌ Tidak dapat terhubung ke Discord: %v", err)
	}
	defer dg.Close()

	log.Println("✅ Bot siap digunakan.")

	// Wait for termination signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

// Event: Bot ready
func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("✅ Bot telah login sebagai %s\n", s.State.User.String())

	// Simpan daftar server ke MongoDB
	guilds := s.State.Guilds
	var servers []interface{}
	for _, guild := range guilds {
		servers = append(servers, bson.M{
			"guild_id":    guild.ID,
			"guild_name":  guild.Name,
			"member_count": len(guild.Members),
		})
	}
	collection := clientInstance.Database(dbName).Collection(serversCol)
	_, err := collection.InsertMany(context.TODO(), servers)
	if err != nil {
		log.Printf("❌ Gagal menyimpan server ke MongoDB: %v\n", err)
	} else {
		log.Println("✅ Daftar server berhasil disimpan ke MongoDB.")
	}
}

// Event: Bot bergabung ke server baru
func onGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	log.Printf("🔔 Bot bergabung ke server baru: %s (%s)", event.Guild.Name, event.Guild.ID)

	// Tambahkan server ke MongoDB
	collection := clientInstance.Database(dbName).Collection(serversCol)
	_, err := collection.InsertOne(context.TODO(), bson.M{
		"guild_id":    event.Guild.ID,
		"guild_name":  event.Guild.Name,
		"member_count": len(event.Guild.Members),
	})
	if err != nil {
		log.Printf("❌ Gagal menyimpan server ke MongoDB: %v", err)
	} else {
		log.Println("✅ Server berhasil disimpan ke MongoDB.")
	}
}

// Event: Bot keluar dari server
func onGuildDelete(s *discordgo.Session, event *discordgo.GuildDelete) {
	log.Printf("🔔 Bot keluar dari server: %s (%s)", event.Guild.Name, event.Guild.ID)

	// Hapus server dari MongoDB
	collection := clientInstance.Database(dbName).Collection(serversCol)
	_, err := collection.DeleteOne(context.TODO(), bson.M{"guild_id": event.Guild.ID})
	if err != nil {
		log.Printf("❌ Gagal menghapus server dari MongoDB: %v", err)
	} else {
		log.Println("✅ Server berhasil dihapus dari MongoDB.")
	}
}

// Event: Slash command
func onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "create-webhook":
		createWebhook(s, i)
	case "list-webhooks":
		listWebhooks(s, i)
	case "server-stats":
		serverStats(s, i)
	}
}

// Command: Create webhook
func createWebhook(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var webhookName string
	if len(options) > 0 {
		webhookName = options[0].StringValue()
	} else {
		webhookName = "DefaultWebhookName"
	}

	webhook, err := s.WebhookCreate(i.ChannelID, webhookName, "")
	if err != nil {
		log.Printf("❌ Gagal membuat webhook: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Gagal membuat webhook.",
			},
		})
		return
	}

	// Simpan webhook ke MongoDB
	collection := clientInstance.Database(dbName).Collection(webhooksCol)
	_, err = collection.InsertOne(context.TODO(), bson.M{
		"guild_id":      i.GuildID,
		"channel_id":    i.ChannelID,
		"webhook_id":    webhook.ID,
		"webhook_name":  webhook.Name,
		"webhook_token": webhook.Token,
	})
	if err != nil {
		log.Printf("❌ Gagal menyimpan webhook ke MongoDB: %v", err)
	} else {
		log.Println("✅ Webhook berhasil disimpan ke MongoDB.")
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Webhook '%s' berhasil dibuat.", webhook.Name),
		},
	})
}

// Command: List webhooks
func listWebhooks(s *discordgo.Session, i *discordgo.InteractionCreate) {
	collection := clientInstance.Database(dbName).Collection(webhooksCol)
	cursor, err := collection.Find(context.TODO(), bson.M{"guild_id": i.GuildID})
	if err != nil {
		log.Printf("❌ Gagal membaca webhooks dari MongoDB: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Gagal membaca daftar webhook.",
			},
		})
		return
	}
	defer cursor.Close(context.TODO())

	var webhooks []bson.M
	if err := cursor.All(context.TODO(), &webhooks); err != nil {
		log.Printf("❌ Gagal mengurai daftar webhooks: %v", err)
		return
	}

	if len(webhooks) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🔍 Tidak ada webhook yang ditemukan untuk server ini.",
			},
		})
		return
	}

	var desc strings.Builder
	for _, webhook := range webhooks {
		desc.WriteString(fmt.Sprintf("**Channel:** %s\n- %s\n", webhook["channel_id"], webhook["webhook_name"]))
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: desc.String(),
		},
	})
}

// Command: Server stats
func serverStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	collection := clientInstance.Database(dbName).Collection(serversCol)
	cursor, err := collection.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Printf("❌ Gagal membaca data server dari MongoDB: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Gagal membaca statistik server.",
			},
		})
		return
	}
	defer cursor.Close(context.TODO())

	var servers []bson.M
	if err := cursor.All(context.TODO(), &servers); err != nil {
		log.Printf("❌ Gagal mengurai data server: %v", err)
		return
	}

	var desc strings.Builder
	for i, server := range servers {
		desc.WriteString(fmt.Sprintf("%d. **%s** (ID: %s, Members: %v)\n", i+1, server["guild_name"], server["guild_id"], server["member_count"]))
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: desc.String(),
		},
	})
}
