package coderd

import (
	"net/http"
	"time"

	"github.com/go-chi/render"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpmw"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username"`
}

type users struct {
	Database database.Store
}

func (users *users) getAuthenticatedUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.User(r)

	render.JSON(rw, r, User{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		Username:  user.Username,
	})
}
