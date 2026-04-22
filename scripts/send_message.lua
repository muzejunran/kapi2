-- Send message request script for wrk
wrk.method = "POST"
wrk.body   = '{"message":"测试消息","page_context":"home"}'
wrk.headers["Content-Type"] = "application/json"

-- Generate different messages for each request
messages = {
    "你好",
    "昨天星巴克38元",
    "这个月外卖花了多少？",
    "帮我分析一下预算",
    "查询我的账单",
    "添加一笔支出",
}

request = function()
    message = messages[math.random(1, #messages)]
    wrk.body = '{"message":"' .. message .. '","page_context":"home"}'
    return wrk.format()
end