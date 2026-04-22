-- Concurrent sessions request script for wrk
wrk.method = "POST"
wrk.body   = '{"user_id":"user_","page_context":"home"}'
wrk.headers["Content-Type"] = "application/json"

-- Generate unique session IDs for concurrent testing
counter = 0

request = function()
    counter = counter + 1
    user_id = "user_" .. counter
    wrk.body = '{"user_id":"' .. user_id .. '","page_context":"home"}'
    return wrk.format()
end
