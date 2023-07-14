package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
)

// connect db
func ConnectDB() *mongo.Client {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file", err.Error())
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(os.Getenv("MONGO_URI")))

	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	err = client.Connect(ctx)

	if err != nil {
		log.Fatal(err)
	}

	// ping database
	err = client.Ping(ctx, nil)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to MongoDB")

	return client
}

// client instance
var DB *mongo.Client = ConnectDB()

// getting database collection
func GetCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	collection := client.Database("changestreamfcm").Collection(collectionName)

	return collection
}

// models
type Notification struct{
	ID primitive.ObjectID `json:"id"`
	Title string `json:"title"`
	Content string `json:"content"`
}

// input
type NotificationPayload struct{
	Title string `json:"title" bson:"title"`
	Content string `json:"content" bson:"content"`
}

type FullDocument struct{
	FullDocument Notif `json:"fullDocument" bson:"fullDocument"`
}

// Notif
type Notif struct{
	Title string `json:"title" bson:"title"`
	Content string `json:"content" bson:"content"`
}

type OID struct{
	OID primitive.ObjectID `json:"$oid" bson:"$oid"`
}

////////////Firebase config

func SetupFirebase() (*firebase.App, context.Context, *messaging.Client) {

    ctx := context.Background()

    serviceAccountKeyFilePath, err := filepath.Abs("./changestream-private-key.json")
    if err != nil {
        panic("Unable to load serviceAccountKeys.json file")
    }

    opt := option.WithCredentialsFile(serviceAccountKeyFilePath)

    //Firebase admin SDK initialization
    app, err := firebase.NewApp(context.Background(), nil, opt)
    if err != nil {
        panic("Firebase load error")
    }

    //Messaging client
    client, _ := app.Messaging(ctx)

    return app, ctx, client
}

func sendToToken(app *firebase.App, Message NotificationPayload) {
	ctx := context.Background()
	client, err := app.Messaging(ctx)
	if err != nil {
		log.Fatalf("error getting Messaging client: %v\n", err)
	}

	registrationToken := os.Getenv("Registration_Token")

	message := &messaging.Message{
		Notification: &messaging.Notification{
			Title: Message.Title,
			Body:  Message.Content,
		},
		Token: registrationToken,
	}

	response, err := client.Send(ctx, message)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("Successfully sent message:", response)
}



func main() {
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowHeaders:     "Origin,Content-Type,Accept,Content-Length,Accept-Language,Accept-Encoding,Connection,Access-Control-Allow-Origin,Authorization",
		AllowOrigins:     "*",
		AllowCredentials: true,
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
	}))

	ConnectDB()

	var noficationCollection *mongo.Collection = GetCollection(DB,"notification")

	// firebase
	appFirebase, _, _ := SetupFirebase()
	
	app.Post("/add", func (c *fiber.Ctx) error {

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()


		var input NotificationPayload

		if err := c.BodyParser(&input); err != nil {
			c.Status(http.StatusBadRequest).JSON(&fiber.Map{"status" : http.StatusBadRequest, "data" : err.Error()})
			return nil
		}


		newNotification := Notification{
			ID : primitive.NewObjectID(),
			Title: input.Title,
			Content: input.Content,
		}

		result, err := noficationCollection.InsertOne(ctx, newNotification)

		if err != nil {
			c.Status(http.StatusBadRequest).JSON(&fiber.Map{"status" : http.StatusBadRequest, "data" : err.Error()})
			return nil
		}

		c.Status(http.StatusOK).JSON(&fiber.Map{"status" : http.StatusOK, "data" : result})
		return nil
	})

	changeStream, err := noficationCollection.Watch(context.TODO(), mongo.Pipeline{})
	if err != nil {
		panic(err)
	}

	go func (){
		for changeStream.Next(context.TODO()) {
			var notif FullDocument
			change := changeStream.Current

			elementJson, err := bson.MarshalExtJSON(change, false, false)
			

			if err != nil{
				panic(err)
			}
		

			err = json.Unmarshal(elementJson, &notif)

			if err != nil{
				panic(err)
			}

			var newNotif NotificationPayload
			newNotif.Title = notif.FullDocument.Title
			newNotif.Content = notif.FullDocument.Content

			sendToToken(appFirebase, newNotif)

		
		}
	}()




	app.Listen(":5000")



	
}