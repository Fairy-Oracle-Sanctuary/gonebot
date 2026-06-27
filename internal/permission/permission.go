// Package permission 三级权限模型.
package permission

import (
	"fmt"
	"sync"
)

// Level 权限等级.
type Level int

const (
	User      Level = iota // 普通用户
	Admin                  // 管理员
	SuperUser              // 超级用户
)

// String 返回权限名.
func (l Level) String() string {
	switch l {
	case User:
		return "user"
	case Admin:
		return "admin"
	case SuperUser:
		return "superuser"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// ParseLevel 解析权限字符串.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "", "user":
		return User, nil
	case "admin":
		return Admin, nil
	case "superuser", "super", "owner":
		return SuperUser, nil
	default:
		return User, fmt.Errorf("unknown permission: %q", s)
	}
}

// Checker 权限检查器.
type Checker struct {
	mu            sync.RWMutex
	superUserIDs  map[int64]struct{}
	adminGroupIDs map[int64]struct{}
}

// NewChecker 创建检查器.
func NewChecker(superUserIDs, adminGroupIDs []int64) *Checker {
	c := &Checker{
		superUserIDs:  make(map[int64]struct{}),
		adminGroupIDs: make(map[int64]struct{}),
	}
	for _, id := range superUserIDs {
		c.superUserIDs[id] = struct{}{}
	}
	for _, id := range adminGroupIDs {
		c.adminGroupIDs[id] = struct{}{}
	}
	return c
}

// Check 检查用户是否满足 required 权限.
//
//	userID: 触发者
//	groupID: 所在群 (0 表示私聊)
//	role: OneBot sender.role
//	required: 命令所需最低权限
func (c *Checker) Check(userID, groupID int64, role string, required Level) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if required == User {
		return true
	}
	if _, ok := c.superUserIDs[userID]; ok {
		return true
	}
	if required == SuperUser {
		return false
	}
	if role == "owner" || role == "admin" {
		return true
	}
	if _, ok := c.adminGroupIDs[groupID]; ok {
		return true
	}
	return false
}

// GetLevel 返回用户的当前有效权限级别.
func (c *Checker) GetLevel(userID, groupID int64, role string) Level {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.superUserIDs[userID]; ok {
		return SuperUser
	}
	if role == "owner" || role == "admin" {
		return Admin
	}
	if _, ok := c.adminGroupIDs[groupID]; ok {
		return Admin
	}
	return User
}

// IsSuperUser 判断是否为超级用户.
func (c *Checker) IsSuperUser(userID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.superUserIDs[userID]
	return ok
}

// SetSuperUsers 替换超级用户列表.
func (c *Checker) SetSuperUsers(ids []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.superUserIDs = make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		c.superUserIDs[id] = struct{}{}
	}
}

// SetAdminGroups 替换管理员群列表.
func (c *Checker) SetAdminGroups(ids []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adminGroupIDs = make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		c.adminGroupIDs[id] = struct{}{}
	}
}

// AddSuperUser 添加超级用户.
func (c *Checker) AddSuperUser(userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.superUserIDs[userID] = struct{}{}
}

// RemoveSuperUser 移除超级用户.
func (c *Checker) RemoveSuperUser(userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.superUserIDs, userID)
}

// AddAdminGroup 添加管理员群.
func (c *Checker) AddAdminGroup(groupID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adminGroupIDs[groupID] = struct{}{}
}

// RemoveAdminGroup 移除管理员群.
func (c *Checker) RemoveAdminGroup(groupID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.adminGroupIDs, groupID)
}

// SuperUserIDs 返回当前超级用户列表快照.
func (c *Checker) SuperUserIDs() []int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]int64, 0, len(c.superUserIDs))
	for id := range c.superUserIDs {
		out = append(out, id)
	}
	return out
}

// ParseLevelSimple 是 ParseLevel 的 panic-free 版本.
func ParseLevelSimple(s string) Level {
	lvl, _ := ParseLevel(s)
	return lvl
}
