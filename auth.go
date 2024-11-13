package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/julienschmidt/httprouter"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// JWT claims
type Claims struct {
	Username string `json:"username"`
	UserID   string `json:"userId"`
	jwt.RegisteredClaims
}

type User struct {
	Username string
	UserID   string
	Password string
}

func login(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Look for the user in MongoDB by username
	var storedUser User
	err := userCollection.FindOne(context.TODO(), bson.M{"username": user.Username}).Decode(&storedUser)
	if err != nil {
		// Return generic error message for security reasons
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(storedUser.Password), []byte(user.Password)); err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// Create JWT claims
	claims := &Claims{
		Username: storedUser.Username,
		UserID:   storedUser.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)), // Adjust expiration as needed
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Cache the token in Redis (only cache if login is successful)
	err = RdxHset("tokki", storedUser.UserID, tokenString)
	if err != nil {
		// Log the Redis caching failure, but allow the login to proceed
		log.Printf("Error caching token in Redis: %v", err)
	}

	// Send response with the token
	sendResponse(w, http.StatusOK, map[string]string{"token": tokenString, "userid": storedUser.UserID}, "Login successful", nil)
}

func register(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}
	log.Printf("Registering user: %s", user.Username)

	// User not found in Redis, check the database
	var existingUser User
	err := userCollection.FindOne(context.TODO(), bson.M{"username": user.Username}).Decode(&existingUser)
	if err == nil {
		// User already exists in database
		log.Printf("User already exists (in DB): %s", user.Username)
		http.Error(w, "User already exists", http.StatusConflict)
		return
	} else if err != mongo.ErrNoDocuments {
		// Handle unexpected error from MongoDB
		log.Printf("Error checking for existing user in DB: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Proceed with password hashing if user does not already exist
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password for user %s: %v", user.Username, err)
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}
	user.Password = string(hashedPassword)
	user.UserID = "u" + GenerateName(10)

	// Insert new user into the database
	_, err = userCollection.InsertOne(context.TODO(), user)
	if err != nil {
		log.Printf("Failed to insert user into DB: %v", err)
		http.Error(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	// Cache the user information in Redis (optional for fast future access)
	err = RdxHset("users", user.Username, user.UserID)
	if err != nil {
		log.Printf("Error caching user in Redis: %v", err)
	}

	// Respond with success
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"status":  http.StatusCreated,
		"message": "User registered successfully",
		"data":    user.Username,
	}
	json.NewEncoder(w).Encode(response)
}

type contextKey string

const userIDKey contextKey = "userId"

// Authenticate middleware
func authenticate(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			http.Error(w, "Missing token", http.StatusUnauthorized)
			return
		}

		if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
			http.Error(w, "Invalid token format", http.StatusUnauthorized)
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString[7:], claims, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Store UserID in context
		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next(w, r.WithContext(ctx), ps) // Call the next handler with new context
	}
}
