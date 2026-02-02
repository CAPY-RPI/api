package router

import (
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// New creates a new chi router with all routes configured
func New(h *handler.Handler, queries *database.Queries, jwtSecret string) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.CORS)

	// Health check (public)
	r.Get("/health", h.Health)

	// API v1 routes
	r.Route("/v1", func(r chi.Router) {
		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Get("/google", h.GoogleAuth)
			r.Get("/google/callback", h.GoogleCallback)
			r.Get("/microsoft", h.MicrosoftAuth)
			r.Get("/microsoft/callback", h.MicrosoftCallback)

			// Protected auth routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.Auth(jwtSecret))
				r.Get("/me", h.GetMe)
				r.Post("/logout", h.Logout)
				r.Post("/refresh", h.RefreshToken)
			})
		})

		// Protected routes - require human authentication
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtSecret))

			// Users
			r.Route("/users", func(r chi.Router) {
				r.Get("/{uid}", h.GetUser)
				r.Put("/{uid}", h.UpdateUser)
				r.Delete("/{uid}", h.DeleteUser)
				r.Get("/{uid}/organizations", h.GetUserOrganizations)
				r.Get("/{uid}/events", h.GetUserEvents)
			})

			// Organizations
			r.Route("/organizations", func(r chi.Router) {
				r.Get("/", h.ListOrganizations)
				r.Post("/", h.CreateOrganization)
				r.Get("/{oid}", h.GetOrganization)
				r.Put("/{oid}", h.UpdateOrganization)
				r.Delete("/{oid}", h.DeleteOrganization)
				r.Get("/{oid}/members", h.ListOrgMembers)
				r.Post("/{oid}/members", h.AddOrgMember)
				r.Delete("/{oid}/members/{uid}", h.RemoveOrgMember)
				r.Get("/{oid}/events", h.ListOrgEvents)
			})

			// Events
			r.Route("/events", func(r chi.Router) {
				r.Get("/", h.ListEvents)
				r.Post("/", h.CreateEvent)
				r.Get("/org/{oid}", h.ListEventsByOrg)
				r.Get("/{eid}", h.GetEvent)
				r.Put("/{eid}", h.UpdateEvent)
				r.Delete("/{eid}", h.DeleteEvent)
				r.Get("/{eid}/registrations", h.ListEventRegistrations)
				r.Post("/{eid}/register", h.RegisterForEvent)
				r.Delete("/{eid}/register", h.UnregisterFromEvent)
			})

			// Bot token management (human auth only)
			r.Route("/bot/tokens", func(r chi.Router) {
				r.Get("/", h.ListBotTokens)
				r.Post("/", h.CreateBotToken)
				r.Delete("/{token_id}", h.RevokeBotToken)
			})
		})

		// Bot routes (M2M auth)
		r.Group(func(r chi.Router) {
			r.Use(middleware.M2MAuth(queries))

			r.Get("/bot/me", h.GetBotMe)

			// Bots can access all the same resources as humans
			r.Route("/bot", func(r chi.Router) {
				// Users (read-only for bots)
				r.Get("/users/{uid}", h.GetUser)
				r.Get("/users/{uid}/organizations", h.GetUserOrganizations)
				r.Get("/users/{uid}/events", h.GetUserEvents)

				// Organizations (full access)
				r.Get("/organizations", h.ListOrganizations)
				r.Post("/organizations", h.CreateOrganization)
				r.Get("/organizations/{oid}", h.GetOrganization)
				r.Put("/organizations/{oid}", h.UpdateOrganization)
				r.Delete("/organizations/{oid}", h.DeleteOrganization)
				r.Get("/organizations/{oid}/members", h.ListOrgMembers)
				r.Post("/organizations/{oid}/members", h.AddOrgMember)
				r.Delete("/organizations/{oid}/members/{uid}", h.RemoveOrgMember)

				// Events (full access)
				r.Get("/events", h.ListEvents)
				r.Post("/events", h.CreateEvent)
				r.Get("/events/{eid}", h.GetEvent)
				r.Put("/events/{eid}", h.UpdateEvent)
				r.Delete("/events/{eid}", h.DeleteEvent)
				r.Get("/events/{eid}/registrations", h.ListEventRegistrations)
				r.Post("/events/{eid}/register", h.RegisterForEvent)
				r.Delete("/events/{eid}/register", h.UnregisterFromEvent)
			})
		})
	})

	return r
}
