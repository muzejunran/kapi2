package main

import (
	"embed"
	"net/http"
	"os"

	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/skillserver"

	"github.com/sirupsen/logrus"
)

//go:embed configs/*.json
var configsFS embed.FS

func main() {
	logger.Setup()
	log := logger.New()

	port := os.Getenv("SKILL_SERVER_PORT")
	if port == "" {
		port = "8090"
	}

	var billRepo *repository.BillRepository
	var budgetRepo *repository.BudgetRepository
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		br, err := repository.NewBillRepository(dsn)
		if err != nil {
			log.WithError(err).Warn("mysql unavailable, bill ops will fallback to mock")
		} else {
			billRepo = br
		}
		bgr, err := repository.NewBudgetRepository(dsn)
		if err != nil {
			log.WithError(err).Warn("mysql unavailable, budget ops will fallback to mock")
		} else {
			budgetRepo = bgr
		}
		if billRepo != nil || budgetRepo != nil {
			log.WithFields(logrus.Fields{
				"bill_repo":   billRepo != nil,
				"budget_repo": budgetRepo != nil,
			}).Info("mysql connected")
		}
	} else {
		log.Warn("MYSQL_DSN not set, using mock data")
	}

	executor := skillserver.NewExecutor(billRepo, budgetRepo)

	srv, err := skillserver.NewServer(configsFS, "configs", executor)
	if err != nil {
		log.WithError(err).Fatal("failed to init skill-server")
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Wrap mux with trace middleware so every request gets a trace_id
	handler := traceMiddleware(mux)

	log.WithField("port", port).Info("skill-server listening")
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.WithError(err).Fatal("server error")
	}
}

// traceMiddleware extracts X-Trace-ID from the request header (or generates one)
// and injects it into the request context so all downstream logs carry it.
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = logger.NewID()
		}
		w.Header().Set("X-Trace-ID", traceID)
		ctx := logger.WithTraceID(r.Context(), traceID)

		logger.FromContext(ctx).WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
		}).Info("[skill-server] request")

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
