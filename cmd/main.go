package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/pusher/pusher-http-go"
	"github.com/rs/cors"
	"go.mongodb.org/mongo-driver/bson" // Import bson for MongoDB filtering
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	client       *mongo.Client
	pusherClient *pusher.Client // Correct type for Pusher client
	collection   *mongo.Collection
)

type DataPoint struct {
	ID    string    `json:"id"`
	Value int       `json:"value"`
	Time  time.Time `json:"time"`
}

func init() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// MongoDB setup
	mongoURI := os.Getenv("MONGODB_URI")
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	collection = client.Database("analytics").Collection("data")

	// Pusher setup
	pusherClient = &pusher.Client{
		AppID:  os.Getenv("PUSHER_APP_ID"),
		Key:    os.Getenv("PUSHER_KEY"),
		Secret: os.Getenv("PUSHER_SECRET"),
		Host:   os.Getenv("PUSHER_CLUSTER") + ".pusher.com",
		Secure: true,
	}
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/data", getData).Methods("GET")
	router.HandleFunc("/data", postData).Methods("POST")

	// CORS setup
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Allow all origins
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
	})
	handler := c.Handler(router)

	log.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

// CORS middleware to handle Cross-Origin Resource Sharing
func addCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins; change to specific origin if needed
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func getData(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	addCORSHeaders(w)

	if r.Method == http.MethodOptions {
		return // Handle preflight request
	}

	var results []DataPoint
	cursor, err := collection.Find(context.TODO(), bson.M{}) // Use bson.M{} instead of nil
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.TODO())

	for cursor.Next(context.TODO()) {
		var data DataPoint
		if err := cursor.Decode(&data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		results = append(results, data)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func postData(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	addCORSHeaders(w)

	if r.Method == http.MethodOptions {
		return // Handle preflight request
	}

	var data DataPoint
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data.Time = time.Now()
	_, err := collection.InsertOne(context.TODO(), data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Trigger a Pusher event
	err = pusherClient.Trigger("analytics-channel", "new-data", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(data)
}
