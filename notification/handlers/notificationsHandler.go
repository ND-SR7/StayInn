package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"notification/clients"
	"notification/data"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NotificationsHandler struct {
	logger            *log.Logger
	repo              *data.NotificationsRepo
	reservationClient clients.ReservationClient
	profileClient     clients.ProfileClient
}

var secretKey = []byte("stayinn_secret")

// Injecting the logger makes this code much more testable
func NewNotificationsHandler(l *log.Logger, r *data.NotificationsRepo, rc clients.ReservationClient, p clients.ProfileClient) *NotificationsHandler {
	return &NotificationsHandler{l, r, rc, p}
}

// TODO Handler methods

func (rh *NotificationsHandler) GetAccommodationRatings(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    accommodationID := vars["idAccommodation"]

    objectID, err := primitive.ObjectIDFromHex(accommodationID)
    if err != nil {
        http.Error(w, "Invalid accommodation ID", http.StatusBadRequest)
        rh.logger.Println("Invalid accommodation ID:", err)
        return
    }

    ratings, err := rh.repo.GetRatingsByAccommodationID(objectID)
    if err != nil {
        http.Error(w, "Failed to fetch ratings", http.StatusBadRequest)
        return
    }

    if err := json.NewEncoder(w).Encode(ratings); err != nil {
        rh.logger.Println("Error encoding accommodation ratings:", err)
        http.Error(w, "Error encoding accommodation ratings", http.StatusInternalServerError)
        return
    }
}

func (rh *NotificationsHandler) AddRating(w http.ResponseWriter, r *http.Request) {
	var rating data.RatingAccommodation
	err := json.NewDecoder(r.Body).Decode(&rating)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing data: %s", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5000*time.Millisecond)
	defer cancel()

	// idAccommodation := rating.IDAccommodation

	tokenStr := rh.extractTokenFromHeader(r)
	guestUsername, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}
	rating.GuestUsername = guestUsername
	rating.Time = time.Now()

	host, err := rh.profileClient.GetUsernameById(ctx, rating.HostID, tokenStr)
	if err != nil {
		rh.logger.Println(err)
		http.Error(w, "Failed to get host", http.StatusBadRequest)
		return
	}

	rating.HostUsername = host.Username

	if rating.Rate < 1 || rating.Rate > 5 {
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), guestUsername, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	rating.GuestID = id

	// reservations, err := rh.reservationClient.GetReservationsByUserIDExp(r.Context(), id)
	// if err != nil {
	// 	http.Error(w, fmt.Sprintf("Error fetching user reservations: %s", err), http.StatusBadRequest)
	// 	return
	// }

	// found := false
	// for _, reservation := range reservations {
	// 	if reservation.IDAccommodation == idAccommodation {
	// 		found = true
	// 		break
	// 	}
	// }

	// if !found {
	// 	http.Error(w, "Accommodation ID not found in user reservations", http.StatusBadRequest)
	// 	return
	// }

	ratings, err := rh.repo.GetAllAccommodationRatingsByUser(r.Context(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching user ratings accommodation: %s", err), http.StatusBadRequest)
		return
	}

	for _, r := range ratings {
		if r.IDAccommodation == rating.IDAccommodation {
			rh.repo.UpdateRatingAccommodationByID(r.ID, id, rating.Rate)
			http.Error(w, "Rating successfully added", http.StatusCreated)
			return
		}
	}

	err = rh.repo.AddRating(&rating)
	if err != nil {
		http.Error(w, "Error adding rating", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Rating successfully added"))
}

func (rh *NotificationsHandler) FindRatingById(rw http.ResponseWriter, h *http.Request) {
	vars := mux.Vars(h)
	ratingID := vars["id"]

	ctx := h.Context()

	objectID, err := primitive.ObjectIDFromHex(ratingID)
	if err != nil {
		http.Error(rw, "Invalid rating ID", http.StatusBadRequest)
		rh.logger.Println("Invalid rating ID:", err)
		return
	}

	rating, err := rh.repo.FindRatingById(ctx, objectID)
	if err != nil {
		rh.logger.Println("Database exception: ", err)
		http.Error(rw, "Database exception", http.StatusInternalServerError)
		return
	}

	if rating == nil {
		rh.logger.Println("No period with given ID in accommodation")
		http.Error(rw, "Rating not found", http.StatusNotFound)
		return
	}

	err = rating.ToJSON(rw)
	if err != nil {
		http.Error(rw, "Unable to convert to json", http.StatusInternalServerError)
		rh.logger.Fatal("Unable to convert to json:", err)
		return
	}
}

func (rh *NotificationsHandler) FindAccommodationRatingByGuest(rw http.ResponseWriter, h *http.Request) {
	vars := mux.Vars(h)
	ratingID := vars["idAccommodation"]

	ctx := h.Context()

	objectID, err := primitive.ObjectIDFromHex(ratingID)
	if err != nil {
		http.Error(rw, "Invalid rating ID", http.StatusBadRequest)
		rh.logger.Println("Invalid rating ID:", err)
		return
	}

	tokenStr := rh.extractTokenFromHeader(h)
	guestUsername, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(rw, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	guestId, err := rh.profileClient.GetUserId(ctx, guestUsername, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to retrive user id from profile service:", err)
		http.Error(rw, "Failed to retrive user id from profile service", http.StatusBadRequest)
		return
	}

	guestIdObject, err := primitive.ObjectIDFromHex(guestId)
	if err != nil {
		rh.logger.Println("Failed to parse id to primitive object id:", err)
		http.Error(rw, "Failed to parse id to primitive object id", http.StatusBadRequest)
		return
	}

	rating, err := rh.repo.FindAccommodationRatingByGuest(ctx, objectID, guestIdObject)
	if err != nil {
		rh.logger.Println("Database exception: ", err)
		http.Error(rw, "Database exception", http.StatusInternalServerError)
		return
	}

	if rating == nil {
		rh.logger.Println("No period with given ID in accommodation")
		http.Error(rw, "Rating not found", http.StatusNotFound)
		return
	}

	err = rating.ToJSON(rw)
	if err != nil {
		http.Error(rw, "Unable to convert to json", http.StatusInternalServerError)
		rh.logger.Fatal("Unable to convert to json:", err)
		return
	}
}

func (rh *NotificationsHandler) FindHostRatingByGuest(rw http.ResponseWriter, h *http.Request) {
	var userId data.UserId
	err := json.NewDecoder(h.Body).Decode(&userId)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Error parsing data: %s", err), http.StatusBadRequest)
		return
	}

	ctx := h.Context()

	tokenStr := rh.extractTokenFromHeader(h)
	guestUsername, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(rw, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	guestId, err := rh.profileClient.GetUserId(ctx, guestUsername, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to retrive user id from profile service:", err)
		http.Error(rw, "Failed to retrive user id from profile service", http.StatusBadRequest)
		return
	}

	guestIdObject, err := primitive.ObjectIDFromHex(guestId)
	if err != nil {
		rh.logger.Println("Failed to parse id to primitive object id:", err)
		http.Error(rw, "Failed to parse id to primitive object id", http.StatusBadRequest)
		return
	}

	rating, err := rh.repo.FindHostRatingByGuest(ctx, userId.ID, guestIdObject)
	if err != nil {
		rh.logger.Println("Database exception: ", err)
		http.Error(rw, "Database exception", http.StatusInternalServerError)
		return
	}

	if rating == nil {
		rh.logger.Println("No period with given ID in accommodation")
		http.Error(rw, "Rating not found", http.StatusNotFound)
		return
	}

	err = rating.ToJSON(rw)
	if err != nil {
		http.Error(rw, "Unable to convert to json", http.StatusInternalServerError)
		rh.logger.Fatal("Unable to convert to json:", err)
		return
	}
}

func (rh *NotificationsHandler) FindHostRatingById(rw http.ResponseWriter, h *http.Request) {
	vars := mux.Vars(h)
	ratingID := vars["id"]

	ctx := h.Context()

	objectID, err := primitive.ObjectIDFromHex(ratingID)
	if err != nil {
		http.Error(rw, "Invalid rating ID", http.StatusBadRequest)
		rh.logger.Println("Invalid rating ID:", err)
		return
	}

	rating, err := rh.repo.FindHostRatingById(ctx, objectID)
	if err != nil {
		rh.logger.Println("Database exception: ", err)
		http.Error(rw, "Database exception", http.StatusInternalServerError)
		return
	}

	if rating == nil {
		rh.logger.Println("No period with given ID in accommodation")
		http.Error(rw, "Rating not found", http.StatusNotFound)
		return
	}

	err = rating.ToJSON(rw)
	if err != nil {
		http.Error(rw, "Unable to convert to json", http.StatusInternalServerError)
		rh.logger.Fatal("Unable to convert to json:", err)
		return
	}
}

func (rh *NotificationsHandler) GetAllAccommodationRatings(w http.ResponseWriter, r *http.Request) {
	ratings, err := rh.repo.GetAllAccommodationRatings(r.Context())
	if err != nil {
		rh.logger.Println("Error fetching all host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(ratings); err != nil {
		rh.logger.Println("Error encoding host ratings:", err)
		http.Error(w, "Error encoding host ratings", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) GetAllAccommodationRatingsForLoggedHost(w http.ResponseWriter, r *http.Request) {
	tokenStr := rh.extractTokenFromHeader(r)
	hostUsername, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	hostId, err := rh.profileClient.GetUserId(ctx, hostUsername, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to retrive user id from profile service:", err)
		http.Error(w, "Failed to retrive user id from profile service", http.StatusBadRequest)
		return
	}

	hostIdObject, err := primitive.ObjectIDFromHex(hostId)
	if err != nil {
		rh.logger.Println("Failed to parse id to primitive object:", err)
		http.Error(w, "Failed to parse id to primitive object", http.StatusBadRequest)
		return
	}

	ratings, err := rh.repo.GetAllAccommodationRatingsForLoggedHost(r.Context(), hostIdObject)
	if err != nil {
		rh.logger.Println("Error fetching all host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(ratings); err != nil {
		rh.logger.Println("Error encoding host ratings:", err)
		http.Error(w, "Error encoding host ratings", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) GetAllAccommodationRatingsByUser(w http.ResponseWriter, r *http.Request) {
	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	ratings, err := rh.repo.GetAllAccommodationRatingsByUser(r.Context(), id)
	if err != nil {
		rh.logger.Println("Error fetching all host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(ratings); err != nil {
		rh.logger.Println("Error encoding host ratings:", err)
		http.Error(w, "Error encoding host ratings", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) GetAllHostRatings(w http.ResponseWriter, r *http.Request) {
	ratings, err := rh.repo.GetAllHostRatings(r.Context())
	if err != nil {
		rh.logger.Println("Error fetching all host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(ratings); err != nil {
		rh.logger.Println("Error encoding host ratings:", err)
		http.Error(w, "Error encoding host ratings", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) GetAllHostRatingsByUser(w http.ResponseWriter, r *http.Request) {
	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}
	ratings, err := rh.repo.GetAllHostRatingsByUser(r.Context(), id)
	if err != nil {
		rh.logger.Println("Error fetching all host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(ratings); err != nil {
		rh.logger.Println("Error encoding host ratings:", err)
		http.Error(w, "Error encoding host ratings", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) GetHostRatings(w http.ResponseWriter, r *http.Request) {
	tokenStr := rh.extractTokenFromHeader(r)
	vars := mux.Vars(r)
	hostUsername, ok := vars["hostUsername"]
	if !ok {
		http.Error(w, "Missing host username in the request path", http.StatusBadRequest)
		return
	}

	_, err := rh.profileClient.GetUserId(r.Context(), hostUsername, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	// id, err := primitive.ObjectIDFromHex(hostID)
	// if err != nil {
	//     http.Error(w, "Invalid hostID", http.StatusBadRequest)
	//     return
	// }

	ratings, err := rh.repo.GetHostRatings(r.Context(), hostUsername)
	if err != nil {
		rh.logger.Println("Error fetching host ratings:", err)
		http.Error(w, "Error fetching host ratings", http.StatusInternalServerError)
		return
	}

	// Convert ratings to JSON and send the response
	err = json.NewEncoder(w).Encode(ratings)
	if err != nil {
		rh.logger.Println("Error encoding response:", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

func (rh *NotificationsHandler) AddHostRating(w http.ResponseWriter, r *http.Request) {
	var rating data.RatingHost
	ctx, cancel := context.WithTimeout(r.Context(), 5000*time.Millisecond)
	defer cancel()

	err := json.NewDecoder(r.Body).Decode(&rating)
	if err != nil {
		http.Error(w, "Error parsing data", http.StatusBadRequest)
		return
	}

	tokenStr := rh.extractTokenFromHeader(r)
	guestUsername, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	host, err := rh.profileClient.GetUsernameById(ctx, rating.HostID, tokenStr)
	if err != nil {
		rh.logger.Println(err)
		http.Error(w, "Failed to get host", http.StatusBadRequest)
		return
	}

	guestId, err := rh.profileClient.GetUserId(ctx, guestUsername, tokenStr)
	if err != nil {
		rh.logger.Println(err)
		http.Error(w, "Failed to get guest", http.StatusBadRequest)
		return
	}

	rating.GuestUsername = guestUsername
	rating.HostUsername = host.Username

	rating.GuestID, err = primitive.ObjectIDFromHex(guestId)
	if err != nil {
		rh.logger.Println(err)
		http.Error(w, "Failed to parse primitive object id", http.StatusBadRequest)
		return
	}

	if rating.Rate < 1 || rating.Rate > 5 {
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	//hostID, err := rh.profileClient.GetUserId(r.Context(), rating.HostUsername)
	//if err != nil {
	//	rh.logger.Println("Failed to get HostID from username:", err)
	//	http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
	//	return
	//}
	//
	//id, err := primitive.ObjectIDFromHex(hostID)
	//if err != nil {
	//	http.Error(w, "Invalid hostID", http.StatusBadRequest)
	//	return
	//}
	//
	//rating.GuestID = id

	//hasExpiredReservations, err := rh.reservationClient.GetReservationsByUserIDExp(r.Context(), rating.GuestID)
	//if err != nil {
	//	http.Error(w, "Error checking expired reservations", http.StatusBadRequest)
	//	return
	//}
	//
	//if len(hasExpiredReservations) == 0 {
	//	http.Error(w, "Guest does not have any expired reservations with the specified host", http.StatusBadRequest)
	//	return
	//}

	rating.Time = time.Now()

	ratings, err := rh.repo.GetAllHostRatingsByUser(r.Context(), rating.HostID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching user ratings host: %s", err), http.StatusBadRequest)
		return
	}

	for _, r := range ratings {
		if r.HostUsername == rating.HostUsername && r.GuestUsername == rating.GuestUsername {
			rh.repo.UpdateHostRating(r.ID, rating.GuestID, &rating)
			http.Error(w, "Host rating successfully added", http.StatusCreated)
			return
		}
	}

	err = rh.repo.AddHostRating(&rating)
	if err != nil {
		http.Error(w, "Error adding host rating", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Host rating successfully added"))
}

func (rh *NotificationsHandler) GetAverageAccommodationRating(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	accommodationID := params["accommodationID"]

	objectID, err := primitive.ObjectIDFromHex(accommodationID)
	if err != nil {
		http.Error(w, "Invalid rating ID", http.StatusBadRequest)
		rh.logger.Println("Invalid rating ID:", err)
		return
	}

	ratings, err := rh.repo.GetRatingsByAccommodationID(objectID)
	if err != nil {
		http.Error(w, "Failed to fetch ratings", http.StatusBadRequest)
		return
	}

	totalRatings := len(ratings)
	if totalRatings == 0 {
		http.Error(w, "No ratings found for this accommodation", http.StatusNotFound)
		return
	}

	sum := 0
	for _, rating := range ratings {
		sum += rating.Rate
	}

	averageRating := float64(sum) / float64(totalRatings)

	avgRatingAccommodation := data.AverageRatingAccommodation{
		AccommodationID: objectID,
		AverageRating:   averageRating,
	}

	jsonResponse, err := json.Marshal(avgRatingAccommodation)
	if err != nil {
		http.Error(w, "Error encoding average rating", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)

}

func (rh *NotificationsHandler) GetAverageHostRating(w http.ResponseWriter, r *http.Request) {
	tokenStr := rh.extractTokenFromHeader(r)
	var userId data.UserId
	err := json.NewDecoder(r.Body).Decode(&userId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing data: %s", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	host, err := rh.profileClient.GetUsernameById(ctx, userId.ID, tokenStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error trying to find user by id: %s", err), http.StatusBadRequest)
		return
	}

	ratings, err := rh.repo.GetRatingsByHostUsername(host.Username)
	if err != nil {
		http.Error(w, "Failed to fetch ratings", http.StatusBadRequest)
		return
	}

	totalRatings := len(ratings)
	if totalRatings == 0 {
		http.Error(w, "No ratings found for this accommodation", http.StatusNotFound)
		return
	}

	sum := 0
	for _, rating := range ratings {
		sum += rating.Rate
	}

	averageRating := float64(sum) / float64(totalRatings)

	avgRatingAccommodation := data.AverageRatingHost{
		Username:      host.Username,
		AverageRating: averageRating,
	}

	jsonResponse, err := json.Marshal(avgRatingAccommodation)
	if err != nil {
		http.Error(w, "Error encoding average rating", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)

}

func (rh *NotificationsHandler) UpdateHostRating(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ratingID, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing rating ID in the request path", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(ratingID)
	if err != nil {
		http.Error(w, "Invalid rating ID", http.StatusBadRequest)
		return
	}

	var newRating data.RatingHost
	if err := json.NewDecoder(r.Body).Decode(&newRating); err != nil {
		http.Error(w, "Error parsing data", http.StatusBadRequest)
		return
	}

	newRating.Time = time.Now()

	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	idUser, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	if err := rh.repo.UpdateHostRating(id, idUser, &newRating); err != nil {
		http.Error(w, "Error updating host rating", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Host rating successfully updated"))
}

func (rh *NotificationsHandler) UpdateAccommodationRating(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ratingID, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing rating ID in the request path", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(ratingID)
	if err != nil {
		http.Error(w, "Invalid rating ID", http.StatusBadRequest)
		return
	}

	var newRating data.RatingAccommodation
	if err := json.NewDecoder(r.Body).Decode(&newRating); err != nil {
		http.Error(w, "Error parsing data", http.StatusBadRequest)
		return
	}

	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	idUser, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	if err := rh.repo.UpdateRatingAccommodationByID(id, idUser, newRating.Rate); err != nil {
		http.Error(w, "Error updating host rating", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Host rating successfully updated"))
}

func (rh *NotificationsHandler) DeleteHostRating(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idParam, ok := vars["id"]

	if !ok {
		http.Error(w, "Missing ID parameter", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	idUser, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	if err := rh.repo.DeleteHostRating(id, idUser); err != nil {
		http.Error(w, "Error deleting host rating", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Host rating successfully deleted"))

}

func (rh *NotificationsHandler) DeleteRatingAccommodationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idParam, ok := vars["id"]

	if !ok {
		http.Error(w, "Missing ID parameter", http.StatusBadRequest)
		return
	}

	id, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	tokenStr := rh.extractTokenFromHeader(r)
	username, err := rh.getUsername(tokenStr)
	if err != nil {
		rh.logger.Println("Failed to read username from token:", err)
		http.Error(w, "Failed to read username from token", http.StatusBadRequest)
		return
	}

	userID, err := rh.profileClient.GetUserId(r.Context(), username, tokenStr)
	if err != nil {
		rh.logger.Println("Failed to get HostID from username:", err)
		http.Error(w, "Failed to get HostID from username", http.StatusBadRequest)
		return
	}

	idUser, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Invalid userID", http.StatusBadRequest)
		return
	}

	err = rh.repo.DeleteRatingAccommodationByID(id, idUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Document deleted successfully"))
}

func (rh *NotificationsHandler) extractTokenFromHeader(rr *http.Request) string {
	token := rr.Header.Get("Authorization")
	if token != "" {
		return token[len("Bearer "):]
	}
	return ""
}

func (rh *NotificationsHandler) getUsername(tokenString string) (string, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})

	if err != nil || !token.Valid {
		return "", err
	}

	username, ok1 := claims["username"].(string)
	_, ok2 := claims["role"].(string)
	if !ok1 || !ok2 {
		return "", err
	}

	return username, nil
}

func (rh *NotificationsHandler) MiddlewareContentTypeSet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, h *http.Request) {
		rw.Header().Add("Content-Type", "application/json")

		next.ServeHTTP(rw, h)
	})
}
