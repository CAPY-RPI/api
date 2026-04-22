package router

import (
	_ "github.com/capyrpi/api/docs/swagger" // Import generated docs
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func mountProtectedRoutes(r chi.Router, h *handler.Handler, jwtSecret string) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(jwtSecret))

		r.Route("/users", func(r chi.Router) {
			r.Get("/{uid}", h.GetUser)
			r.Put("/{uid}", h.UpdateUser)
			r.Delete("/{uid}", h.DeleteUser)
			r.Get("/{uid}/organizations", h.GetUserOrganizations)
			r.Get("/{uid}/events", h.GetUserEvents)
		})

		// Links
		r.Route("/links", func(r chi.Router) {
			r.Post("/", h.CreateLink)
			r.Put("/{lid}", h.UpdateLink)
			r.Get("/{lid}/visits", h.GetTotalVisits)
			r.Get("/{lid}/qrcode", h.GetQRCode)
		})

		r.Route("/bot/tokens", func(r chi.Router) {
			r.Get("/", h.ListBotTokens)
			r.Post("/", h.CreateBotToken)
			r.Delete("/{token_id}", h.RevokeBotToken)
		})
	})

	r.With(middleware.Auth(jwtSecret)).Post("/organizations", h.CreateOrganization)
	r.With(middleware.Auth(jwtSecret)).Get("/organizations/{oid}", h.GetOrganization)
	r.With(middleware.Auth(jwtSecret)).Put("/organizations/{oid}", h.UpdateOrganization)
	r.With(middleware.Auth(jwtSecret)).Delete("/organizations/{oid}", h.DeleteOrganization)
	r.With(middleware.Auth(jwtSecret)).Get("/organizations/{oid}/members", h.ListOrgMembers)
	r.With(middleware.Auth(jwtSecret)).Post("/organizations/{oid}/members", h.AddOrgMember)
	r.With(middleware.Auth(jwtSecret)).Delete("/organizations/{oid}/members/{uid}", h.RemoveOrgMember)
	r.With(middleware.Auth(jwtSecret)).Get("/organizations/{oid}/events", h.ListOrgEvents)
	r.With(middleware.Auth(jwtSecret)).Get("/organizations/{oid}/links", h.ListOrgLinks)

	r.With(middleware.Auth(jwtSecret)).Post("/events", h.CreateEvent)
	r.With(middleware.Auth(jwtSecret)).Get("/events/org/{oid}", h.ListEventsByOrg)
	r.With(middleware.Auth(jwtSecret)).Get("/events/{eid}", h.GetEvent)
	r.With(middleware.Auth(jwtSecret)).Put("/events/{eid}", h.UpdateEvent)
	r.With(middleware.Auth(jwtSecret)).Delete("/events/{eid}", h.DeleteEvent)
	r.With(middleware.Auth(jwtSecret)).Get("/events/{eid}/registrations", h.ListEventRegistrations)
	r.With(middleware.Auth(jwtSecret)).Post("/events/{eid}/register", h.RegisterForEvent)
	r.With(middleware.Auth(jwtSecret)).Delete("/events/{eid}/register", h.UnregisterFromEvent)
}

// New creates a new chi router with all routes configured
func New(h *handler.Handler, queries database.Querier, jwtSecret string, allowedOrigins []string) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	if h.Config.Env != "bench" {
		r.Use(chimiddleware.Logger)
	}
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.CORS(allowedOrigins, h.Config.Env == "development"))

	// Public link resolution alias.
	r.Get("/r/{endpoint_url}", h.ResolveLink)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Health check (public)
		r.Get("/health", h.Health)

		// Link resolution (public)
		// TODO Use a global variable to link this with links.go
		r.Get("/r/{endpoint_url}", h.ResolveLink)

		// Swagger UI (public) - Only in non-production environments
		if h.Config.Env != "production" {
			r.Get("/swagger/*", httpSwagger.WrapHandler)
		}

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

			// Public read-only collection routes
			r.Get("/organizations", h.ListOrganizations)
			r.Get("/events", h.ListEvents)

			mountProtectedRoutes(r, h, jwtSecret)

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
					r.Post("/organizations", h.CreateBotOrganization)
					r.Get("/organizations/guilds/{guild_id}", h.GetOrganizationByGuildID)
					r.Get("/organizations/{oid}", h.GetOrganization)
					r.Put("/organizations/{oid}", h.UpdateOrganization)
					r.Delete("/organizations/{oid}", h.DeleteOrganization)
					r.Get("/organizations/{oid}/members", h.ListOrgMembers)
					r.Post("/organizations/{oid}/members", h.AddOrgMember)
					r.Delete("/organizations/{oid}/members/{uid}", h.RemoveOrgMember)
					r.Get("/{oid}/links", h.ListOrgLinks)

					// Events (full access)
					r.Get("/events", h.ListEvents)
					r.Post("/events", h.CreateEvent)
					r.Get("/events/{eid}", h.GetEvent)
					r.Put("/events/{eid}", h.UpdateEvent)
					r.Delete("/events/{eid}", h.DeleteEvent)
					r.Get("/events/{eid}/registrations", h.ListEventRegistrations)
					r.Post("/events/{eid}/register", h.RegisterForEvent)
					r.Delete("/events/{eid}/register", h.UnregisterFromEvent)

					// Links
					r.Route("/links", func(r chi.Router) {
						r.Post("/", h.CreateLink)
						r.Put("/{lid}", h.UpdateLink)
						r.Get("/{lid}/visits", h.GetTotalVisits)
						r.Get("/{lid}/qrcode", h.GetQRCode)
					})
				})
			})
		})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Get("/organizations", h.ListOrganizations)
		r.Get("/events", h.ListEvents)
		mountProtectedRoutes(r, h, jwtSecret)
	})

	return r
}
