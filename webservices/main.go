package main

import (
    "fmt"
    "net/http"
    "os"
    "strings"

    "github.com/errata-ai/vale/v3/internal/core"
    "github.com/errata-ai/vale/v3/internal/lint"
    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
)

// ValeRequest defines the structure of the incoming JSON request.
type ValeRequest struct {
    Data string `json:"data"`
}

// ValeLinter holds the Vale linter instance.
type ValeLinter struct {
    linter *lint.Linter
}

// The required API key for accessing the endpoints.
const apiKey = "secret-api-key-48316-48316"

// lintHandler is the Echo handler for the /vale/ endpoint (JSON response).
func (l *ValeLinter) lintHandler(c echo.Context) error {
    req := new(ValeRequest)
    if err := c.Bind(req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    }

    lintedFiles, err := l.linter.LintString(req.Data)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    response := make(map[string][]core.Alert)
    for _, f := range lintedFiles {
        response[f.Path] = f.SortedAlerts()
    }

    return c.JSON(http.StatusOK, response)
}

// lintHandlerText is the Echo handler for the /vale-text/ endpoint (plain text response).
func (l *ValeLinter) lintHandlerText(c echo.Context) error {
    req := new(ValeRequest)
    if err := c.Bind(req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    }

    lintedFiles, err := l.linter.LintString(req.Data)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    // Build a string response that mimics the Vale CLI output.
    var responseBuilder strings.Builder
    var errors, warnings, suggestions int

    for _, f := range lintedFiles {
        alerts := f.SortedAlerts()
        if len(alerts) == 0 {
            continue
        }

        responseBuilder.WriteString(fmt.Sprintf("\n %s\n", f.Path))
        responseBuilder.WriteString(strings.Repeat("-", len(f.Path)+2) + "\n")

        for _, a := range alerts {
            switch a.Severity {
            case "error":
                errors++
            case "warning":
                warnings++
            case "suggestion":
                suggestions++
            }
            // Format: LINE:COL  SEVERITY  MESSAGE  CHECK
            line := fmt.Sprintf("%d:%-4d %-12s %-25s %s\n", a.Line, a.Span[0], a.Severity, a.Message, a.Check)
            responseBuilder.WriteString(line)
        }
    }

    // Add summary footer
    symbol := "✔"
    if errors > 0 || warnings > 0 {
        symbol = "✖"
    }
    summary := fmt.Sprintf("\n%s %d %s, %d %s and %d %s in %d %s.\n",
        symbol,
        errors, pluralize("error", errors),
        warnings, pluralize("warning", warnings),
        suggestions, pluralize("suggestion", suggestions),
        len(lintedFiles), pluralize("file", len(lintedFiles)))
    responseBuilder.WriteString(summary)

    return c.String(http.StatusOK, responseBuilder.String())
}

// pluralize returns the plural form of a word if n is not 1.
func pluralize(s string, n int) string {
    if n != 1 {
        return s + "s"
    }
    return s
}

func main() {
    // Initialize Vale's configuration and linter once at startup.
    flags := &core.CLIFlags{
        InExt: ".md",
    }

    config, err := core.ReadPipeline(flags, false)
    if err != nil {
        e := echo.New()
        e.Logger.Fatal("Failed to initialize Vale config: ", err)
        os.Exit(1)
    }

    l, err := lint.NewLinter(config)
    if err != nil {
        e := echo.New()
        e.Logger.Fatal("Failed to initialize Vale linter: ", err)
        os.Exit(1)
    }

    valeLinter := &ValeLinter{linter: l}
    e := echo.New()

    // Standard middleware
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())

    // API Key Authentication Middleware
    // This middleware will protect all endpoints defined after it.
    e.Use(middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
        return key == apiKey, nil
    }))

    // Define API endpoints
    e.POST("/vale/", valeLinter.lintHandler)
    e.POST("/vale-text/", valeLinter.lintHandlerText)

    // Start the server
    e.Logger.Info("Starting Vale server on port 1323...")
    e.Logger.Info("Endpoints are protected. Use 'Authorization: Bearer " + apiKey + "'")
    e.Logger.Fatal(e.Start(":1323"))
}