package data

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	// NoSQL: module containing Mongo api client
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	// TODO "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type ReservationRepo struct {
	cli    *mongo.Client
	logger *log.Logger
}

// Constructor
func New(ctx context.Context, logger *log.Logger) (*ReservationRepo, error) {
	dburi := os.Getenv("MONGO_DB_URI")

	client, err := mongo.NewClient(options.Client().ApplyURI(dburi))
	if err != nil {
		return nil, err
	}

	err = client.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return &ReservationRepo{
		cli:    client,
		logger: logger,
	}, nil
}

// Disconnect
func (rr *ReservationRepo) Disconnect(ctx context.Context) error {
	err := rr.cli.Disconnect(ctx)
	if err != nil {
		return err
	}
	return nil
}

// Check database connection
func (rr *ReservationRepo) Ping() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check connection -> if no error, connection is established
	err := rr.cli.Ping(ctx, readpref.Primary())
	if err != nil {
		rr.logger.Println(err)
	}

	// Print available databases
	databases, err := rr.cli.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		rr.logger.Println(err)
	}
	fmt.Println(databases)
}

func (rr *ReservationRepo) GetAll() (Reservations, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reservationCollection := rr.getCollection()

	var reservations Reservations
	reserationCursor, err := reservationCollection.Find(ctx, bson.M{})
	if err != nil {
		rr.logger.Println(err)
		return nil, err
	}
	if err = reserationCursor.All(ctx, &reservations); err != nil {
		rr.logger.Println(err)
		return nil, err
	}
	return reservations, nil
}

func (pr *ReservationRepo) GetById(id string) (*Reservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reservationCollection := pr.getCollection()

	var reservation Reservation
	objID, _ := primitive.ObjectIDFromHex(id)
	err := reservationCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&reservation)
	if err != nil {
		pr.logger.Println(err)
		return nil, err
	}
	return &reservation, nil
}

func (rr *ReservationRepo) Insert(reservation *Reservation) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reservationCollection := rr.getCollection()

	//	reservation.ID = primitive.NewObjectID()

	result, err := reservationCollection.InsertOne(ctx, &reservation)
	if err != nil {
		rr.logger.Println(err)
		return err
	}
	rr.logger.Printf("Reservation Id: %v\n", result.InsertedID)
	return nil
}

func (rr *ReservationRepo) AddAvaiablePeriod(id string, period *AvailabilityPeriod) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resevations := rr.getCollection()

	objID, _ := primitive.ObjectIDFromHex(id)
	filter := bson.D{{Key: "_id", Value: objID}}
	update := bson.M{"$push": bson.M{
		"availabilityPeriods": period,
	}}
	result, err := resevations.UpdateOne(ctx, filter, update)
	rr.logger.Printf("Documents matched: %v\n", result.MatchedCount)
	rr.logger.Printf("Documents updated: %v\n", result.ModifiedCount)

	if err != nil {
		rr.logger.Println(err)
		return err
	}
	return nil
}

func (rr *ReservationRepo) DeleteById(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reservationCollection := rr.getCollection()

	objID, _ := primitive.ObjectIDFromHex(id)
	filter := bson.D{{Key: "_id", Value: objID}}
	result, err := reservationCollection.DeleteOne(ctx, filter)
	if err != nil {
		rr.logger.Println(err)
		return err
	}
	rr.logger.Printf("Reservation deleted: %v\n", result.DeletedCount)
	return nil
}

func (rr *ReservationRepo) getCollection() *mongo.Collection {
	patientDatabase := rr.cli.Database("reservationDB")
	patientCollection := patientDatabase.Collection("reservation")
	return patientCollection
}
