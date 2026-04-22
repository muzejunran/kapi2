-- KAPI Database Initialization Script

-- ============================================
-- Conversations Table
-- 存储用户对话记录
-- ============================================
CREATE TABLE IF NOT EXISTS conversations (
    id VARCHAR(64) PRIMARY KEY COMMENT '会话ID',
    user_id VARCHAR(64) NOT NULL COMMENT '用户ID',
    role VARCHAR(20) NOT NULL COMMENT '角色: user/assistant/system',
    content TEXT NOT NULL COMMENT '对话内容',
    created_at DATETIME NOT NULL COMMENT '创建时间',
    INDEX idx_user_id (user_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户对话记录表';

-- ============================================
-- Bills Table
-- 存储账单记录（用于财务技能）
-- ============================================
CREATE TABLE IF NOT EXISTS bills (
    id VARCHAR(64) PRIMARY KEY COMMENT '账单ID',
    user_id VARCHAR(64) NOT NULL COMMENT '用户ID',
    amount DECIMAL(10,2) NOT NULL COMMENT '金额',
    category VARCHAR(50) NOT NULL COMMENT '分类',
    description TEXT COMMENT '描述',
    timestamp DATETIME NOT NULL COMMENT '账单时间',
    created_at DATETIME NOT NULL COMMENT '创建时间',
    updated_at DATETIME NOT NULL COMMENT '更新时间',
    INDEX idx_user_id (user_id),
    INDEX idx_timestamp (timestamp),
    INDEX idx_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账单记录表';

-- ============================================
-- Health Check Table (Optional)
-- 用于健康检查
-- ============================================
CREATE TABLE IF NOT EXISTS health_check (
    id INT PRIMARY KEY,
    updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO health_check (id, updated_at) VALUES (1, NOW());
