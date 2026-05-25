package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
)

var (
	currentQQ             int64
	myNickname            string
	targetQQ              int64
	targetGroupID         string
	myQQNumber            int64
	clientSeq             int64
	sentCount             int
	pendingLoginQQ        int64
	historyOffset         int
	historyTargetQQ       int64
	historyTargetNickname string
)

func handleCommand(conn *websocket.Conn, text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "/to":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /to <qq_number>")
			return true
		}
		qq, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			fmt.Println("[cmd] invalid QQ number")
			return true
		}
		checkUser(conn, qq)

	case "/register":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /register <password> <nickname>")
			return true
		}
		registerUser(conn, parts[1], parts[2])

	case "/login":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /login <qq_number> <password>")
			return true
		}
		qq, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			fmt.Println("[cmd] invalid QQ number")
			return true
		}
		loginUser(conn, qq, parts[2])

	case "/addfriend":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /addfriend <qq_number> [message]")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		message := ""
		if len(parts) > 2 {
			message = strings.Join(parts[2:], " ")
		}
		addFriend(conn, qqNum, message)

	case "/accept":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /accept <qq_number>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		acceptFriend(conn, qqNum)

	case "/reject":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /reject <qq_number>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		rejectFriend(conn, qqNum)

	case "/delfriend":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /delfriend <qq_number>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		delFriend(conn, qqNum)

	case "/friends":
		listFriends(conn)

	case "/search":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /search <keyword>")
			return true
		}
		searchUser(conn, parts[1])

	case "/movefriend":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /movefriend <qq_number> <group>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		groupName := strings.Join(parts[2:], " ")
		moveFriend(conn, qqNum, groupName)

	case "/groups":
		listGroups(conn)

	case "/remark":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /remark <qq_number> <remark>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		remark := strings.Join(parts[2:], " ")
		remarkFriend(conn, qqNum, remark)

	case "/creategroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /creategroup <name>")
			return true
		}
		createGroup(conn, strings.Join(parts[1:], " "))

	case "/delgroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /delgroup <name>")
			return true
		}
		deleteGroup(conn, strings.Join(parts[1:], " "))

	case "/prev":
		if historyTargetQQ == 0 {
			fmt.Println("[cmd] no chat history context, use /to <qq_number> first")
			return true
		}
		historyOffset += 30
		requestHistory(conn, historyTargetQQ, historyOffset)

	case "/next":
		if historyTargetQQ == 0 {
			fmt.Println("[cmd] no chat history context, use /to <qq_number> first")
			return true
		}
		if historyOffset >= 30 {
			historyOffset -= 30
		} else {
			historyOffset = 0
		}
		requestHistory(conn, historyTargetQQ, historyOffset)

	case "/mkgroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /mkgroup <name>")
			return true
		}
		createChatGroup(conn, strings.Join(parts[1:], " "))

	case "/joingroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /joingroup <group_id>")
			return true
		}
		joinChatGroup(conn, parts[1])

	case "/leavegroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /leavegroup <group_id>")
			return true
		}
		leaveChatGroup(conn, parts[1])

	case "/mygroups":
		listMyGroups(conn)

	case "/togroup":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /togroup <group_id>")
			return true
		}
		switchToGroup(conn, parts[1])

	case "/sessions":
		listSessions(conn)

	case "/who":
		if targetQQ == 0 {
			fmt.Println("[cmd] no target set, use /to <qq_number>")
		} else {
			fmt.Printf("[cmd] chatting with %d\n", targetQQ)
		}

	case "/whoami":
		if myQQNumber == 0 {
			fmt.Println("[cmd] not logged in")
		} else {
			fmt.Printf("[cmd] %s (QQ: %d)\n", myNickname, myQQNumber)
		}

	case "/help":
		fmt.Println("[cmd] Commands:")
		fmt.Println("  /register <password> <nickname>       - create account")
		fmt.Println("  /login <qq_number> <password>         - login")
		fmt.Println("  /to <qq_number>                       - switch chat target")
		fmt.Println("  /who                                  - show current target")
		fmt.Println("  /whoami                               - show current account info")
		fmt.Println("  /addfriend <qq_number> [message]      - send friend request")
		fmt.Println("  /accept <qq_number>                   - accept friend request")
		fmt.Println("  /reject <qq_number>                   - reject friend request")
		fmt.Println("  /delfriend <qq_number>                - delete friend")
		fmt.Println("  /friends                              - list friends")
		fmt.Println("  /search <keyword>                     - search users")
		fmt.Println("  /movefriend <qq_number> <group>       - move friend to group")
		fmt.Println("  /groups                               - list friend groups")
		fmt.Println("  /creategroup <name>                   - create friend group")
		fmt.Println("  /delgroup <name>                      - delete friend group")
		fmt.Println("  /remark <qq_number> <remark>          - set friend remark")
		fmt.Println("  /prev                                 - previous 30 history messages")
		fmt.Println("  /next                                 - next 30 history messages")
		fmt.Println("  /mkgroup <name>                       - create chat group")
		fmt.Println("  /joingroup <group_id>                 - join chat group")
		fmt.Println("  /leavegroup <group_id>                - leave chat group")
		fmt.Println("  /mygroups                             - list my chat groups")
		fmt.Println("  /togroup <group_id>                   - switch to group chat")
		fmt.Println("  /sessions                             - list all chat sessions")
		fmt.Println("  /help                                 - show this help")
		fmt.Println("  /quit                                 - exit")

	case "/quit":
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		time.Sleep(100 * time.Millisecond)
		conn.Close()
		os.Exit(0)

	default:
		fmt.Printf("[cmd] unknown command: %s, type /help for help\n", parts[0])
	}
	return true
}

func registerUser(conn *websocket.Conn, password, nickname string) {
	pendingLoginQQ = 0
	payload, _ := json.Marshal(&model.RegisterRequest{
		Password: password,
		Nickname: nickname,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeRegister,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] register request sent for %s\n", nickname)
}

func addFriend(conn *websocket.Conn, qqNumber int64, message string) {
	payload, _ := json.Marshal(&model.FriendRequestPayload{
		ToQQNumber: qqNumber,
		Message:    message,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendRequest,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] friend request sent to qq=%d\n", qqNumber)
}

func acceptFriend(conn *websocket.Conn, qqNumber int64) {
	payload, _ := json.Marshal(&model.FriendRequestPayload{
		ToQQNumber: qqNumber,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendAccept,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] accepting friend request from qq=%d\n", qqNumber)
}

func rejectFriend(conn *websocket.Conn, qqNumber int64) {
	payload, _ := json.Marshal(&model.FriendRequestPayload{
		ToQQNumber: qqNumber,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendReject,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] rejecting friend request from qq=%d\n", qqNumber)
}

func delFriend(conn *websocket.Conn, qqNumber int64) {
	payload, _ := json.Marshal(&model.FriendRequestPayload{
		ToQQNumber: qqNumber,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendDelete,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] deleting friend qq=%d\n", qqNumber)
}

func listFriends(conn *websocket.Conn) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendList,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func searchUser(conn *websocket.Conn, keyword string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendSearch,
		Content: keyword,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func moveFriend(conn *websocket.Conn, qqNumber int64, groupName string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"qq_number":  qqNumber,
		"group_name": groupName,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendMoveGroup,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] moving friend qq=%d to group '%s'\n", qqNumber, groupName)
}

func listGroups(conn *websocket.Conn) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendGroups,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func remarkFriend(conn *websocket.Conn, qqNumber int64, remark string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"qq_number": qqNumber,
		"remark":    remark,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendRemark,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] setting remark for qq=%d: %s\n", qqNumber, remark)
}

func checkUser(conn *websocket.Conn, qq int64) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeCheckUser,
		Content: strconv.FormatInt(qq, 10),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func requestHistory(conn *websocket.Conn, targetQQ int64, offset int) {
	payload, _ := json.Marshal(&model.HistoryRequest{
		TargetQQ: targetQQ,
		Offset:   offset,
		Limit:    30,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeHistory,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func createChatGroup(conn *websocket.Conn, name string) {
	payload, _ := json.Marshal(&model.GroupCreateRequest{Name: name})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeGroupCreate,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func joinChatGroup(conn *websocket.Conn, groupID string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeGroupJoin,
		Content: groupID,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func leaveChatGroup(conn *websocket.Conn, groupID string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeGroupLeave,
		Content: groupID,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func listMyGroups(conn *websocket.Conn) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeGroupList,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func switchToGroup(conn *websocket.Conn, groupID string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeGroupInfo,
		Content: groupID,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func listSessions(conn *websocket.Conn) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeSessionList,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
}

func createGroup(conn *websocket.Conn, name string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendCreateGroup,
		Content: name,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] creating group '%s'\n", name)
}

func deleteGroup(conn *websocket.Conn, name string) {
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeFriendDeleteGroup,
		Content: name,
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] deleting group '%s'\n", name)
}

func loginUser(conn *websocket.Conn, qq int64, password string) {
	pendingLoginQQ = qq
	payload, _ := json.Marshal(&model.LoginRequest{
		QQ:       qq,
		Password: password,
		Platform: "cli",
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeLogin,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] login request sent for %d\n", qq)
}

func prompt() {
	if myQQNumber == 0 {
		fmt.Print("(not logged in) > ")
	} else if targetGroupID != "" {
		fmt.Printf("[%s QQ:%d -> Group:%s] > ", myNickname, myQQNumber, targetGroupID)
	} else if targetQQ == 0 {
		fmt.Printf("(%s QQ:%d) > ", myNickname, myQQNumber)
	} else {
		fmt.Printf("[%s QQ:%d -> QQ:%d] > ", myNickname, myQQNumber, targetQQ)
	}
}

func main() {
	addr := "ws://localhost:8080/ws"
	fmt.Printf("Connecting to %s...\n", addr)

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	fmt.Println("Welcome to QQGO! Use /login or /register to get started.")
	prompt()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) || websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				log.Printf("[client] read error: %v", err)
				return
			}

			var msg model.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("[client] unmarshal error: %v", err)
				continue
			}

			switch msg.MsgType {
			case model.MsgTypeLoginAck:
				var resp model.LoginResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					if resp.Code == 0 {
						if pendingLoginQQ != 0 {
							currentQQ = pendingLoginQQ
							pendingLoginQQ = 0
						}
						myQQNumber = resp.QQNumber
						myNickname = resp.Nickname
						fmt.Printf("\r[Server]: login ok, %s(QQ:%d), online=%d\n> ", myNickname, myQQNumber, resp.Online)
					} else {
						pendingLoginQQ = 0
						fmt.Printf("\r[Server]: login failed - %s\n> ", resp.Message)
					}
				}

			case model.MsgTypeRegisterAck:
				var resp model.RegisterResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					if resp.Code == 0 {
						fmt.Printf("\r[Server]: register ok, your QQ number is %d\n> ", resp.QQNumber)
					} else {
						fmt.Printf("\r[Server]: %s\n> ", resp.Message)
					}
				}

			case model.MsgTypeServerAck:
				sentCount--
				if sentCount <= 0 {
					sentCount = 0
					fmt.Printf("\r[sent ✓] %s\n> ", msg.Content)
					prompt()
				}

			case model.MsgTypeDelivered:
				fmt.Printf("\r[delivered ✓✓] message #%d\n> ", msg.ClientSeq)
				prompt()

			case model.MsgTypeText:
				fmt.Printf("\r[%d -> %d]: %s\n> ", msg.FromQQ, msg.ToQQ, msg.Content)

				ackPayload, _ := json.Marshal(&model.AckRequest{MessageID: msg.ID})
				ackMsg := &model.Message{
					MsgType: model.MsgTypeDelivered,
					Content: string(ackPayload),
				}
				ackData, _ := json.Marshal(ackMsg)
				conn.WriteMessage(websocket.TextMessage, ackData)
				prompt()

			case model.MsgTypeFriendRequest:
				fmt.Printf("\r[Friend Request] %s\n> ", msg.Content)
				prompt()

			case model.MsgTypeFriendAccept:
				fmt.Printf("\r[Friend Accepted] %s\n> ", msg.Content)
				prompt()

			case model.MsgTypeFriendList:
				var resp model.FriendListResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					displayFriendList(resp.Friends, resp.AllGroups)
				}
				prompt()

			case model.MsgTypeFriendSearch:
				var results []model.UserSearchResult
				if err := json.Unmarshal([]byte(msg.Content), &results); err == nil {
					displaySearchResults(results)
				}
				prompt()

			case model.MsgTypeFriendMoveGroup:
				fmt.Printf("\r[Server]: %s\n> ", msg.Content)
				prompt()

			case model.MsgTypeFriendGroups:
				var resp model.FriendGroupListResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					fmt.Println("\n───── Friend Groups ─────")
					for _, g := range resp.Groups {
						fmt.Printf("  [%s]\n", g)
					}
					fmt.Println("─────────────────────────")
				}
				prompt()

			case model.MsgTypeCheckUser:
				var resp model.CheckUserResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					if resp.Code == 0 {
						targetQQ = resp.QQNumber
						historyTargetQQ = resp.QQNumber
						historyOffset = 0
						statusIcon := "●"
						if !resp.Online {
							statusIcon = "○"
						}
						fmt.Printf("\r[cmd] switched to %s %s(QQ:%d)\n> ", statusIcon, resp.Nickname, resp.QQNumber)
						requestHistory(conn, resp.QQNumber, 0)
					} else {
						fmt.Printf("\r[cmd] %s\n> ", resp.Message)
					}
				}
				prompt()

			case model.MsgTypeHistory:
				var resp model.HistoryResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					historyTargetNickname = resp.Nickname
					displayHistory(resp)
				}
				prompt()

			case model.MsgTypeGroupCreate:
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(msg.Content), &result); err == nil {
					groupID, _ := result["group_id"].(string)
					name, _ := result["name"].(string)
					fmt.Printf("\r[Group] created: %s (ID: %s)\n> ", name, groupID)
				}
				prompt()

			case model.MsgTypeGroupList:
				var resp model.GroupListResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					displayGroupList(resp.Groups)
				}
				prompt()

			case model.MsgTypeGroupInfo:
				var info model.GroupInfo
				if err := json.Unmarshal([]byte(msg.Content), &info); err == nil {
					targetGroupID = info.GroupID
					targetQQ = 0
					historyTargetQQ = 0
					fmt.Printf("\r[cmd] switched to group: %s (ID: %s, members: %d)\n> ", info.Name, info.GroupID, info.MemberCnt)
				}
				prompt()

			case model.MsgTypeSessionList:
				var resp model.SessionListResponse
				if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
					displaySessionList(resp.Sessions)
				}
				prompt()

			default:
				fmt.Printf("\r[%d]: %s\n> ", msg.FromQQ, msg.Content)
				prompt()
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type /help for commands, /to <qq_number> to start chatting:")
	prompt()

	for scanner.Scan() {
		select {
		case <-interrupt:
			return
		default:
		}

		text := scanner.Text()
		if text == "" {
			prompt()
			continue
		}

		if strings.HasPrefix(text, "/") {
			handleCommand(conn, text)
			prompt()
			continue
		}

		if targetQQ == 0 && targetGroupID == "" {
			fmt.Println("[cmd] no target set, use /to <qq_number> or /togroup <group_id> first")
			prompt()
			continue
		}

		clientSeq++
		sentCount++
		msg := &model.Message{
			ClientSeq: clientSeq,
			MsgType:   model.MsgTypeText,
			FromQQ:    currentQQ,
			ToQQ:      targetQQ,
			GroupID:   targetGroupID,
			Content:   text,
		}

		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[client] write error: %v", err)
			return
		}
		prompt()
	}
}

func displayFriendList(friends []model.FriendInfo, allGroups []string) {
	fmt.Println("\n───── Friend List ─────")

	grouped := make(map[string][]model.FriendInfo)
	for _, f := range friends {
		grouped[f.GroupName] = append(grouped[f.GroupName], f)
	}

	orderGroups := []string{"待处理", "我的好友"}
	displayed := make(map[string]bool)

	for _, g := range orderGroups {
		if list, ok := grouped[g]; ok {
			fmt.Printf("\n  [%s]\n", g)
			for _, f := range list {
				statusIcon := "●"
				if !f.Online {
					statusIcon = "○"
				}
				displayName := f.Nickname
				if f.Remark != "" {
					displayName = f.Remark + "(" + f.Nickname + ")"
				}
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QQNumber, displayName)
			}
			displayed[g] = true
		} else if g == "待处理" {
		} else {
			fmt.Printf("\n  [%s]\n", g)
			displayed[g] = true
		}
	}

	for _, g := range allGroups {
		if displayed[g] {
			continue
		}
		fmt.Printf("\n  [%s]\n", g)
		if list, ok := grouped[g]; ok {
			for _, f := range list {
				statusIcon := "●"
				if !f.Online {
					statusIcon = "○"
				}
				displayName := f.Nickname
				if f.Remark != "" {
					displayName = f.Remark + "(" + f.Nickname + ")"
				}
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QQNumber, displayName)
			}
		}
		displayed[g] = true
	}

	for g, list := range grouped {
		if !displayed[g] {
			fmt.Printf("\n  [%s]\n", g)
			for _, f := range list {
				statusIcon := "●"
				if !f.Online {
					statusIcon = "○"
				}
				displayName := f.Nickname
				if f.Remark != "" {
					displayName = f.Remark + "(" + f.Nickname + ")"
				}
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QQNumber, displayName)
			}
		}
	}

	fmt.Println("──────────────────────")
}

func displaySearchResults(results []model.UserSearchResult) {
	fmt.Println("\n───── Search Results ─────")
	if len(results) == 0 {
		fmt.Println("  (no results)")
	} else {
		for _, r := range results {
			statusIcon := "○"
			if r.Online {
				statusIcon = "●"
			}
			fmt.Printf("  %s QQ:%d  %s\n", statusIcon, r.QQNumber, r.Nickname)
		}
	}
	fmt.Println("──────────────────────────")
}

func displayHistory(resp model.HistoryResponse) {
	if len(resp.Messages) == 0 && resp.Offset == 0 {
		return
	}

	fmt.Printf("\n───── History with %s (QQ:%d) ─────\n", resp.Nickname, resp.TargetQQ)
	for _, m := range resp.Messages {
		timeStr := m.CreatedAt.Format("15:04:05")
		if m.FromQQ == myQQNumber {
			fmt.Printf("  [我]    %s  %s\n", timeStr, m.Content)
		} else {
			fmt.Printf("  [%s] %s  %s\n", resp.Nickname, timeStr, m.Content)
		}
	}
	if resp.HasMore {
		fmt.Println("  ... (use /prev for older messages)")
	}
	if resp.Offset > 0 {
		fmt.Println("  (use /next for newer messages)")
	}
	fmt.Println("─────────────────────────────────────")
}

func displayGroupList(groups []model.GroupInfo) {
	fmt.Println("\n───── My Groups ─────")
	if len(groups) == 0 {
		fmt.Println("  (no groups)")
	} else {
		for _, g := range groups {
			ownerMark := ""
			if g.OwnerQQ == myQQNumber {
				ownerMark = " [owner]"
			}
			fmt.Printf("  %s  %s (members: %d)%s\n", g.GroupID, g.Name, g.MemberCnt, ownerMark)
		}
	}
	fmt.Println("─────────────────────")
}

func displaySessionList(sessions []model.SessionInfo) {
	fmt.Println("\n───── Sessions ─────")
	if len(sessions) == 0 {
		fmt.Println("  (no sessions)")
	} else {
		for _, s := range sessions {
			if s.Type == "private" {
				statusIcon := "○"
				if s.Online {
					statusIcon = "●"
				}
				timeStr := s.LastTime.Format("01-02 15:04")
				msg := s.LastMessage
				if len(msg) > 30 {
					msg = msg[:30] + "..."
				}
				fmt.Printf("  %s QQ:%-8d  %-12s  %s  %s\n", statusIcon, s.TargetQQ, s.Nickname, timeStr, msg)
			} else {
				timeStr := s.LastTime.Format("01-02 15:04")
				msg := s.LastMessage
				if len(msg) > 30 {
					msg = msg[:30] + "..."
				}
				fmt.Printf("  # %-16s  %-12s  %s  %s\n", s.GroupID, s.Nickname, timeStr, msg)
			}
		}
	}
	fmt.Println("────────────────────")
}
