// Package bot Bot 聚合根, 提供 OneBot v11 API 调用.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"neobot/core/internal/event"
	"neobot/core/internal/ws"
)

// Bot 与 OneBot 通信的核心.
type Bot struct {
	ws     *ws.Client
	selfID int64
	log    *slog.Logger
}

// New 创建 Bot.
func New(w *ws.Client) *Bot {
	return &Bot{
		ws:  w,
		log: slog.Default().With("module", "bot"),
	}
}

// SetSelfID 设置 self_id.
func (b *Bot) SetSelfID(id int64) { b.selfID = id }

// SelfID 返回 bot QQ 号.
func (b *Bot) SelfID() int64 { return b.selfID }

// WS 返回底层 ws 客户端.
func (b *Bot) WS() *ws.Client { return b.ws }

// ============================================================
// 消息 API (Message)
// ============================================================

// SendPrivateMsg 发送私聊消息.
func (b *Bot) SendPrivateMsg(ctx context.Context, userID int64, message any) (int64, error) {
	b.log.Debug("send_private_msg", "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "send_private_msg", map[string]any{
		"user_id": userID, "message": message,
	})
	if err != nil {
		b.log.Warn("send_private_msg failed", "err", err)
		return 0, err
	}
	if !resp.IsOK() {
		b.log.Warn("send_private_msg api error", "retcode", resp.RetCode, "msg", resp.Msg)
		return 0, fmt.Errorf("send_private_msg failed: %s", resp.Msg)
	}
	var data struct {
		MessageID int64 `json:"message_id"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		b.log.Warn("send_private_msg unmarshal failed", "err", err)
	}
	return data.MessageID, nil
}

// SendGroupMsg 发送群消息.
func (b *Bot) SendGroupMsg(ctx context.Context, groupID int64, message any) (int64, error) {
	b.log.Debug("send_group_msg", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "send_group_msg", map[string]any{
		"group_id": groupID, "message": message,
	})
	if err != nil {
		b.log.Warn("send_group_msg failed", "err", err)
		return 0, err
	}
	if !resp.IsOK() {
		b.log.Warn("send_group_msg api error", "retcode", resp.RetCode, "msg", resp.Msg)
		return 0, fmt.Errorf("send_group_msg failed: %s", resp.Msg)
	}
	var data struct {
		MessageID int64 `json:"message_id"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		b.log.Warn("send_group_msg unmarshal failed", "err", err)
	}
	return data.MessageID, nil
}

// Reply 回复一条消息 (自动判断 group/private).
func (b *Bot) Reply(ctx context.Context, ev *event.MessageEvent, message any) (int64, error) {
	switch {
	case ev.IsGroup():
		return b.SendGroupMsg(ctx, ev.GroupID, message)
	case ev.IsPrivate():
		return b.SendPrivateMsg(ctx, ev.UserID, message)
	default:
		return 0, fmt.Errorf("unknown message_type=%q", ev.MessageType)
	}
}

// DeleteMsg 撤回消息.
func (b *Bot) DeleteMsg(ctx context.Context, messageID int64) error {
	b.log.Debug("delete_msg", "message_id", messageID)
	resp, err := b.ws.CallAPI(ctx, "delete_msg", map[string]any{"message_id": messageID})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("delete_msg failed: %s", resp.Msg)
	}
	return nil
}

// GetMsg 获取一条消息的详细信息.
func (b *Bot) GetMsg(ctx context.Context, messageID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_msg", "message_id", messageID)
	return b.ws.CallAPI(ctx, "get_msg", map[string]any{"message_id": messageID})
}

// GetForwardMsg 获取合并转发消息的内容.
func (b *Bot) GetForwardMsg(ctx context.Context, id string) (*ws.APIResponse, error) {
	b.log.Debug("get_forward_msg", "id", id)
	return b.ws.CallAPI(ctx, "get_forward_msg", map[string]any{"id": id})
}

// SendGroupForwardMsg 发送群聊合并转发消息.
func (b *Bot) SendGroupForwardMsg(ctx context.Context, groupID int64, messages []map[string]any) (*ws.APIResponse, error) {
	b.log.Debug("send_group_forward_msg", "group_id", groupID)
	return b.ws.CallAPI(ctx, "send_group_forward_msg", map[string]any{
		"group_id": groupID, "messages": messages,
	})
}

// SendPrivateForwardMsg 发送私聊合并转发消息.
func (b *Bot) SendPrivateForwardMsg(ctx context.Context, userID int64, messages []map[string]any) (*ws.APIResponse, error) {
	b.log.Debug("send_private_forward_msg", "user_id", userID)
	return b.ws.CallAPI(ctx, "send_private_forward_msg", map[string]any{
		"user_id": userID, "messages": messages,
	})
}

// ============================================================
// 群组 API (Group)
// ============================================================

// GroupKick 踢出群成员.
func (b *Bot) GroupKick(ctx context.Context, groupID, userID int64, reject bool) error {
	b.log.Debug("set_group_kick", "group_id", groupID, "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "set_group_kick", map[string]any{
		"group_id": groupID, "user_id": userID, "reject_add_request": reject,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_kick failed: %s", resp.Msg)
	}
	return nil
}

// GroupBan 禁言群成员.
func (b *Bot) GroupBan(ctx context.Context, groupID, userID int64, duration int) error {
	b.log.Debug("set_group_ban", "group_id", groupID, "user_id", userID, "duration", duration)
	resp, err := b.ws.CallAPI(ctx, "set_group_ban", map[string]any{
		"group_id": groupID, "user_id": userID, "duration": duration,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_ban failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupAnonymousBan 禁言群组中的匿名用户.
func (b *Bot) SetGroupAnonymousBan(ctx context.Context, groupID int64, anonymous map[string]any, flag string, duration int) error {
	b.log.Debug("set_group_anonymous_ban", "group_id", groupID)
	params := map[string]any{"group_id": groupID, "duration": duration}
	if anonymous != nil {
		params["anonymous"] = anonymous
	}
	if flag != "" {
		params["flag"] = flag
	}
	resp, err := b.ws.CallAPI(ctx, "set_group_anonymous_ban", params)
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_anonymous_ban failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupWholeBan 开启或关闭群组全员禁言.
func (b *Bot) SetGroupWholeBan(ctx context.Context, groupID int64, enable bool) error {
	b.log.Debug("set_group_whole_ban", "group_id", groupID, "enable", enable)
	resp, err := b.ws.CallAPI(ctx, "set_group_whole_ban", map[string]any{
		"group_id": groupID, "enable": enable,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_whole_ban failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupAdmin 设置或取消群组成员的管理员权限.
func (b *Bot) SetGroupAdmin(ctx context.Context, groupID, userID int64, enable bool) error {
	b.log.Debug("set_group_admin", "group_id", groupID, "user_id", userID, "enable", enable)
	resp, err := b.ws.CallAPI(ctx, "set_group_admin", map[string]any{
		"group_id": groupID, "user_id": userID, "enable": enable,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_admin failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupAnonymous 开启或关闭群组的匿名聊天功能.
func (b *Bot) SetGroupAnonymous(ctx context.Context, groupID int64, enable bool) error {
	b.log.Debug("set_group_anonymous", "group_id", groupID, "enable", enable)
	resp, err := b.ws.CallAPI(ctx, "set_group_anonymous", map[string]any{
		"group_id": groupID, "enable": enable,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_anonymous failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupCard 设置群组成员的群名片.
func (b *Bot) SetGroupCard(ctx context.Context, groupID, userID int64, card string) error {
	b.log.Debug("set_group_card", "group_id", groupID, "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "set_group_card", map[string]any{
		"group_id": groupID, "user_id": userID, "card": card,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_card failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupName 设置群组的名称.
func (b *Bot) SetGroupName(ctx context.Context, groupID int64, groupName string) error {
	b.log.Debug("set_group_name", "group_id", groupID, "group_name", groupName)
	resp, err := b.ws.CallAPI(ctx, "set_group_name", map[string]any{
		"group_id": groupID, "group_name": groupName,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_name failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupLeave 退出或解散一个群组.
func (b *Bot) SetGroupLeave(ctx context.Context, groupID int64, isDismiss bool) error {
	b.log.Debug("set_group_leave", "group_id", groupID, "is_dismiss", isDismiss)
	resp, err := b.ws.CallAPI(ctx, "set_group_leave", map[string]any{
		"group_id": groupID, "is_dismiss": isDismiss,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_leave failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupSpecialTitle 为群组成员设置专属头衔.
func (b *Bot) SetGroupSpecialTitle(ctx context.Context, groupID, userID int64, specialTitle string, duration int) error {
	b.log.Debug("set_group_special_title", "group_id", groupID, "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "set_group_special_title", map[string]any{
		"group_id": groupID, "user_id": userID, "special_title": specialTitle, "duration": duration,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_special_title failed: %s", resp.Msg)
	}
	return nil
}

// GetGroupInfo 获取群组的详细信息.
func (b *Bot) GetGroupInfo(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_info", "group_id", groupID)
	return b.ws.CallAPI(ctx, "get_group_info", map[string]any{"group_id": groupID})
}

// GetGroupList 获取群列表.
func (b *Bot) GetGroupList(ctx context.Context) ([]map[string]any, error) {
	b.log.Debug("get_group_list")
	resp, err := b.ws.CallAPI(ctx, "get_group_list", nil)
	if err != nil {
		return nil, err
	}
	if !resp.IsOK() {
		return nil, fmt.Errorf("get_group_list failed: %s", resp.Msg)
	}
	var list []map[string]any
	_ = json.Unmarshal(resp.Data, &list)
	return list, nil
}

// GetGroupMemberInfo 获取指定群组成员的详细信息.
func (b *Bot) GetGroupMemberInfo(ctx context.Context, groupID, userID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_member_info", "group_id", groupID, "user_id", userID)
	return b.ws.CallAPI(ctx, "get_group_member_info", map[string]any{
		"group_id": groupID, "user_id": userID,
	})
}

// GetGroupMemberList 获取一个群组的所有成员列表.
func (b *Bot) GetGroupMemberList(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_member_list", "group_id", groupID)
	return b.ws.CallAPI(ctx, "get_group_member_list", map[string]any{"group_id": groupID})
}

// GetGroupHonorInfo 获取群组的荣誉信息.
func (b *Bot) GetGroupHonorInfo(ctx context.Context, groupID int64, honorType string) (*ws.APIResponse, error) {
	b.log.Debug("get_group_honor_info", "group_id", groupID, "type", honorType)
	return b.ws.CallAPI(ctx, "get_group_honor_info", map[string]any{
		"group_id": groupID, "type": honorType,
	})
}

// SetGroupAddRequest 处理加群请求或邀请.
func (b *Bot) SetGroupAddRequest(ctx context.Context, flag, subType string, approve bool, reason string) error {
	b.log.Debug("set_group_add_request", "flag", flag, "sub_type", subType, "approve", approve)
	resp, err := b.ws.CallAPI(ctx, "set_group_add_request", map[string]any{
		"flag": flag, "sub_type": subType, "approve": approve, "reason": reason,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_add_request failed: %s", resp.Msg)
	}
	return nil
}

// GetGroupInfoEx 获取群扩展信息 (NapCat 特有).
func (b *Bot) GetGroupInfoEx(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_info_ex", "group_id", groupID)
	return b.ws.CallAPI(ctx, "get_group_info_ex", map[string]any{"group_id": groupID})
}

// DeleteEssenceMsg 删除精华消息.
func (b *Bot) DeleteEssenceMsg(ctx context.Context, messageID int64) error {
	b.log.Debug("delete_essence_msg", "message_id", messageID)
	resp, err := b.ws.CallAPI(ctx, "delete_essence_msg", map[string]any{"message_id": messageID})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("delete_essence_msg failed: %s", resp.Msg)
	}
	return nil
}

// GroupPoke 在群内发送戳一戳.
func (b *Bot) GroupPoke(ctx context.Context, groupID, userID int64) error {
	b.log.Debug("group_poke", "group_id", groupID, "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "group_poke", map[string]any{
		"group_id": groupID, "user_id": userID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("group_poke failed: %s", resp.Msg)
	}
	return nil
}

// MarkGroupMsgAsRead 标记群消息为已读.
func (b *Bot) MarkGroupMsgAsRead(ctx context.Context, groupID int64) error {
	b.log.Debug("mark_group_msg_as_read", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "mark_group_msg_as_read", map[string]any{
		"group_id": groupID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("mark_group_msg_as_read failed: %s", resp.Msg)
	}
	return nil
}

// ForwardGroupSingleMsg 转发单条群消息.
func (b *Bot) ForwardGroupSingleMsg(ctx context.Context, groupID int64, messageID string) error {
	b.log.Debug("forward_group_single_msg", "group_id", groupID, "message_id", messageID)
	resp, err := b.ws.CallAPI(ctx, "forward_group_single_msg", map[string]any{
		"group_id": groupID, "message_id": messageID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("forward_group_single_msg failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupPortrait 设置群头像.
func (b *Bot) SetGroupPortrait(ctx context.Context, groupID int64, file string, cache int) error {
	b.log.Debug("set_group_portrait", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "set_group_portrait", map[string]any{
		"group_id": groupID, "file": file, "cache": cache,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_portrait failed: %s", resp.Msg)
	}
	return nil
}

// SendGroupNotice 发送群公告.
func (b *Bot) SendGroupNotice(ctx context.Context, groupID int64, content string) error {
	b.log.Debug("_send_group_notice", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "_send_group_notice", map[string]any{
		"group_id": groupID, "content": content,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("_send_group_notice failed: %s", resp.Msg)
	}
	return nil
}

// GetGroupNotice 获取群公告.
func (b *Bot) GetGroupNotice(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("_get_group_notice", "group_id", groupID)
	return b.ws.CallAPI(ctx, "_get_group_notice", map[string]any{"group_id": groupID})
}

// DelGroupNotice 删除群公告.
func (b *Bot) DelGroupNotice(ctx context.Context, groupID int64, noticeID string) error {
	b.log.Debug("_del_group_notice", "group_id", groupID, "notice_id", noticeID)
	resp, err := b.ws.CallAPI(ctx, "_del_group_notice", map[string]any{
		"group_id": groupID, "notice_id": noticeID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("_del_group_notice failed: %s", resp.Msg)
	}
	return nil
}

// GetGroupAtAllRemain 获取 @全体成员 的剩余次数.
func (b *Bot) GetGroupAtAllRemain(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_at_all_remain", "group_id", groupID)
	return b.ws.CallAPI(ctx, "get_group_at_all_remain", map[string]any{"group_id": groupID})
}

// GetGroupSystemMsg 获取群系统消息.
func (b *Bot) GetGroupSystemMsg(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_group_system_msg")
	return b.ws.CallAPI(ctx, "get_group_system_msg", nil)
}

// GetGroupShutList 获取群禁言列表.
func (b *Bot) GetGroupShutList(ctx context.Context, groupID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_group_shut_list", "group_id", groupID)
	return b.ws.CallAPI(ctx, "get_group_shut_list", map[string]any{"group_id": groupID})
}

// SetGroupRemark 设置群备注.
func (b *Bot) SetGroupRemark(ctx context.Context, groupID int64, remark string) error {
	b.log.Debug("set_group_remark", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "set_group_remark", map[string]any{
		"group_id": groupID, "remark": remark,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_remark failed: %s", resp.Msg)
	}
	return nil
}

// SetGroupSign 设置群签到.
func (b *Bot) SetGroupSign(ctx context.Context, groupID int64) error {
	b.log.Debug("set_group_sign", "group_id", groupID)
	resp, err := b.ws.CallAPI(ctx, "set_group_sign", map[string]any{"group_id": groupID})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_group_sign failed: %s", resp.Msg)
	}
	return nil
}

// ============================================================
// 好友 API (Friend)
// ============================================================

// SendLike 点赞.
func (b *Bot) SendLike(ctx context.Context, userID int64, times int) error {
	b.log.Debug("send_like", "user_id", userID, "times", times)
	resp, err := b.ws.CallAPI(ctx, "send_like", map[string]any{
		"user_id": userID, "times": times,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("send_like failed: %s", resp.Msg)
	}
	return nil
}

// GetStrangerInfo 获取陌生人的信息.
func (b *Bot) GetStrangerInfo(ctx context.Context, userID int64) (*ws.APIResponse, error) {
	b.log.Debug("get_stranger_info", "user_id", userID)
	return b.ws.CallAPI(ctx, "get_stranger_info", map[string]any{"user_id": userID})
}

// GetFriendList 获取好友列表.
func (b *Bot) GetFriendList(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_friend_list")
	return b.ws.CallAPI(ctx, "get_friend_list", nil)
}

// SetFriendAddRequest 处理加好友请求.
func (b *Bot) SetFriendAddRequest(ctx context.Context, flag string, approve bool, remark string) error {
	b.log.Debug("set_friend_add_request", "flag", flag, "approve", approve)
	resp, err := b.ws.CallAPI(ctx, "set_friend_add_request", map[string]any{
		"flag": flag, "approve": approve, "remark": remark,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_friend_add_request failed: %s", resp.Msg)
	}
	return nil
}

// GetFriendsWithCategory 获取带分类的好友列表.
func (b *Bot) GetFriendsWithCategory(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_friends_with_category")
	return b.ws.CallAPI(ctx, "get_friends_with_category", nil)
}

// GetUnidirectionalFriendList 获取单向好友列表.
func (b *Bot) GetUnidirectionalFriendList(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_unidirectional_friend_list")
	return b.ws.CallAPI(ctx, "get_unidirectional_friend_list", nil)
}

// FriendPoke 发送好友戳一戳.
func (b *Bot) FriendPoke(ctx context.Context, userID int64) error {
	b.log.Debug("friend_poke", "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "friend_poke", map[string]any{"user_id": userID})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("friend_poke failed: %s", resp.Msg)
	}
	return nil
}

// MarkPrivateMsgAsRead 标记私聊消息为已读.
func (b *Bot) MarkPrivateMsgAsRead(ctx context.Context, userID int64) error {
	b.log.Debug("mark_private_msg_as_read", "user_id", userID)
	resp, err := b.ws.CallAPI(ctx, "mark_private_msg_as_read", map[string]any{
		"user_id": userID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("mark_private_msg_as_read failed: %s", resp.Msg)
	}
	return nil
}

// GetFriendMsgHistory 获取私聊消息历史记录.
func (b *Bot) GetFriendMsgHistory(ctx context.Context, userID int64, count int) (*ws.APIResponse, error) {
	b.log.Debug("get_friend_msg_history", "user_id", userID, "count", count)
	return b.ws.CallAPI(ctx, "get_friend_msg_history", map[string]any{
		"user_id": userID, "count": count,
	})
}

// ForwardFriendSingleMsg 转发单条好友消息.
func (b *Bot) ForwardFriendSingleMsg(ctx context.Context, userID int64, messageID string) error {
	b.log.Debug("forward_friend_single_msg", "user_id", userID, "message_id", messageID)
	resp, err := b.ws.CallAPI(ctx, "forward_friend_single_msg", map[string]any{
		"user_id": userID, "message_id": messageID,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("forward_friend_single_msg failed: %s", resp.Msg)
	}
	return nil
}

// ============================================================
// 账号 API (Account)
// ============================================================

// GetLoginInfo 获取登录信息.
func (b *Bot) GetLoginInfo(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_login_info")
	return b.ws.CallAPI(ctx, "get_login_info", nil)
}

// GetVersionInfo 获取版本信息.
func (b *Bot) GetVersionInfo(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_version_info")
	return b.ws.CallAPI(ctx, "get_version_info", nil)
}

// GetStatus 获取状态信息.
func (b *Bot) GetStatus(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_status")
	return b.ws.CallAPI(ctx, "get_status", nil)
}

// BotExit 让机器人进程退出.
func (b *Bot) BotExit(ctx context.Context) error {
	b.log.Debug("bot_exit")
	resp, err := b.ws.CallAPI(ctx, "bot_exit", nil)
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("bot_exit failed: %s", resp.Msg)
	}
	return nil
}

// SetSelfLongnick 设置个性签名.
func (b *Bot) SetSelfLongnick(ctx context.Context, longNick string) error {
	b.log.Debug("set_self_longnick")
	resp, err := b.ws.CallAPI(ctx, "set_self_longnick", map[string]any{
		"longNick": longNick,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_self_longnick failed: %s", resp.Msg)
	}
	return nil
}

// SetInputStatus 设置"对方正在输入..."状态.
func (b *Bot) SetInputStatus(ctx context.Context, userID int64, eventType int) error {
	b.log.Debug("set_input_status", "user_id", userID, "event_type", eventType)
	resp, err := b.ws.CallAPI(ctx, "set_input_status", map[string]any{
		"user_id": userID, "event_type": eventType,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_input_status failed: %s", resp.Msg)
	}
	return nil
}

// SetDiyOnlineStatus 设置自定义在线状态.
func (b *Bot) SetDiyOnlineStatus(ctx context.Context, faceID, faceType int, wording string) error {
	b.log.Debug("set_diy_online_status")
	resp, err := b.ws.CallAPI(ctx, "set_diy_online_status", map[string]any{
		"face_id": faceID, "face_type": faceType, "wording": wording,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_diy_online_status failed: %s", resp.Msg)
	}
	return nil
}

// SetOnlineStatus 设置在线状态.
func (b *Bot) SetOnlineStatus(ctx context.Context, statusCode int) error {
	b.log.Debug("set_online_status", "status_code", statusCode)
	resp, err := b.ws.CallAPI(ctx, "set_online_status", map[string]any{
		"status_code": statusCode,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_online_status failed: %s", resp.Msg)
	}
	return nil
}

// SetQQProfile 设置个人资料.
func (b *Bot) SetQQProfile(ctx context.Context, params map[string]any) error {
	b.log.Debug("set_qq_profile")
	resp, err := b.ws.CallAPI(ctx, "set_qq_profile", params)
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_qq_profile failed: %s", resp.Msg)
	}
	return nil
}

// SetQQAvatar 设置头像.
func (b *Bot) SetQQAvatar(ctx context.Context, params map[string]any) error {
	b.log.Debug("set_qq_avatar")
	resp, err := b.ws.CallAPI(ctx, "set_qq_avatar", params)
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("set_qq_avatar failed: %s", resp.Msg)
	}
	return nil
}

// GetClientkey 获取客户端密钥.
func (b *Bot) GetClientkey(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_clientkey")
	return b.ws.CallAPI(ctx, "get_clientkey", nil)
}

// CleanCache 清理缓存.
func (b *Bot) CleanCache(ctx context.Context) error {
	b.log.Debug("clean_cache")
	resp, err := b.ws.CallAPI(ctx, "clean_cache", nil)
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("clean_cache failed: %s", resp.Msg)
	}
	return nil
}

// GetProfileLike 获取个人资料点赞信息.
func (b *Bot) GetProfileLike(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("get_profile_like")
	return b.ws.CallAPI(ctx, "get_profile_like", nil)
}

// NcGetUserStatus 获取用户在线状态 (NapCat 特有).
func (b *Bot) NcGetUserStatus(ctx context.Context, userID int64) (*ws.APIResponse, error) {
	b.log.Debug("nc_get_user_status", "user_id", userID)
	return b.ws.CallAPI(ctx, "nc_get_user_status", map[string]any{"user_id": userID})
}

// ============================================================
// 媒体 API (Media)
// ============================================================

// CanSendImage 检查是否可以发送图片.
func (b *Bot) CanSendImage(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("can_send_image")
	return b.ws.CallAPI(ctx, "can_send_image", nil)
}

// CanSendRecord 检查是否可以发送语音.
func (b *Bot) CanSendRecord(ctx context.Context) (*ws.APIResponse, error) {
	b.log.Debug("can_send_record")
	return b.ws.CallAPI(ctx, "can_send_record", nil)
}

// GetImage 获取图片信息.
func (b *Bot) GetImage(ctx context.Context, file string) (*ws.APIResponse, error) {
	b.log.Debug("get_image", "file", file)
	return b.ws.CallAPI(ctx, "get_image", map[string]any{"file": file})
}

// GetFile 获取文件信息.
func (b *Bot) GetFile(ctx context.Context, fileID string) (*ws.APIResponse, error) {
	b.log.Debug("get_file", "file_id", fileID)
	return b.ws.CallAPI(ctx, "get_file", map[string]any{"file_id": fileID})
}

// CallAPI 通用 API 调用.
func (b *Bot) CallAPI(ctx context.Context, action string, params map[string]any) (*ws.APIResponse, error) {
	return b.ws.CallAPI(ctx, action, params)
}
