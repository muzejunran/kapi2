-- Health check request script for wrk
wrk.method = "GET"
wrk.body   = nil
wrk.path   = "/health"