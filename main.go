package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Fungsi untuk koneksi ke MongoDB
func connectToMongoDB(uri string) (*mongo.Client, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = client.Connect(ctx)
	if err != nil {
		return nil, err
	}

	// Cek koneksi
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	fmt.Println("Berhasil terhubung ke MongoDB Atlas!")
	return client, nil
}

func main() {
	// MongoDB Atlas URI
	mongoURI := "mongodb+srv://BooKece:Boolua@cluster0.h9luy.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"

	// Hubungkan ke MongoDB Atlas
	mongoClient, err := connectToMongoDB(mongoURI)
	if err != nil {
		log.Fatalf("Gagal menghubungkan ke MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	// Pilih koleksi database
	collection := mongoClient.Database("discordBotDB").Collection("messages")

	// Token bot Discord
	token := "MTMzMTk0NDAzNzkwODc0MjIwNQ.G2Oqlf.D4jZLzmh0SdMAf5Jm_r1-EX2wxEd8_oJ7AZTgA"

	// Buat instance bot
	bot, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Gagal membuat bot: %v", err)
	}

	// Event handler ketika menerima pesan
	bot.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Hindari memproses pesan dari bot itu sendiri
		if m.Author.ID == s.State.User.ID {
			return
		}

		// Simpan pesan ke MongoDB
		newMessage := map[string]interface{}{
			"user_id":   m.Author.ID,
			"username":  m.Author.Username,
			"content":   m.Content,
			"channelID": m.ChannelID,
			"timestamp": time.Now(),
		}

		_, err := collection.InsertOne(context.Background(), newMessage)
		if err != nil {
			log.Println("Gagal menyimpan pesan ke MongoDB:", err)
		} else {
			fmt.Printf("Pesan dari %s berhasil disimpan ke MongoDB!\n", m.Author.Username)
		}

		// Kirim balasan ke channel
		if m.Content == "!ping" {
			s.ChannelMessageSend(m.ChannelID, "Pong!")
		}
	})

	// Mulai bot
	err = bot.Open()
	if err != nil {
		log.Fatalf("Gagal menghubungkan bot ke Discord: %v", err)
	}
	fmt.Println("Bot berjalan. Tekan CTRL+C untuk keluar.")

	// Menangani penghentian program
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("Bot dimatikan.")
	bot.Close()
}
