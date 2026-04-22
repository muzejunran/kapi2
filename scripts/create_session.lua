-- Create session request script for wrk
wrk.method = "POST"
wrk.body   = '{"user_id":"test_user_","page_context":"home"}'
wrk.headers["Content-Type"] = "application/json"

-- Generate unique user ID for each request
request = function()
    user_id = "user_" .. math.random(100000, 999999)
    wrk.body = '{"user_id":"' .. user_id .. '","page_context":"home"}'
    return wrk.format()
end