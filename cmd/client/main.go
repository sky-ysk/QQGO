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

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
)

var (
	currentUser string
	targetUID   string
	myQQNumber  int64
	clientSeq   int64
	sentCount   int
)

func handleCommand(conn *websocket.Conn, text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "/to":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /to <uid>")
			return true
		}
		targetUID = parts[1]
		fmt.Printf("[cmd] switched to chatting with %s\n", targetUID)

	case "/register":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /register <password> <nickname>")
			return true
		}
		registerUser(conn, currentUser, parts[1], parts[2])

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

	case "/who":
		if targetUID == "" {
			fmt.Println("[cmd] no target set, use /to <uid>")
		} else {
			fmt.Printf("[cmd] chatting with %s\n", targetUID)
		}

	case "/help":
		fmt.Println("[cmd] Commands:")
		fmt.Println("  /register <password> <nickname>     - create account")
		fmt.Println("  /to <uid>                           - switch chat target")
		fmt.Println("  /who                                - show current target")
		fmt.Println("  /addfriend <qq_number> [message]    - send friend request")
		fmt.Println("  /accept <qq_number>                 - accept friend request")
		fmt.Println("  /reject <qq_number>                 - reject friend request")
		fmt.Println("  /delfriend <qq_number>              - delete friend")
		fmt.Println("  /friends                            - list friends")
		fmt.Println("  /search <keyword>                   - search users")
		fmt.Println("  /movefriend <qq_number> <group>     - move friend to group")
		fmt.Println("  /groups                             - list friend groups")
		fmt.Println("  /remark <qq_number> <remark>        - set friend remark")
		fmt.Println("  /help                               - show this help")
		fmt.Println("  /quit                               - exit")

	case "/quit":
		conn.Close()
		os.Exit(0)

	default:
		fmt.Printf("[cmd] unknown command: %s, type /help for help\n", parts[0])
	}
	return true
}

func registerUser(conn *websocket.Conn, uid, password, nickname string) {
	payload, _ := json.Marshal(&model.RegisterRequest{
		UID:      uid,
		Password: password,
		Nickname: nickname,
	})
	msg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeRegister,
		Content: string(payload),
	})
	conn.WriteMessage(websocket.TextMessage, msg)
	fmt.Printf("[cmd] register request sent for %s\n", uid)
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

func login(conn *websocket.Conn, uid, password string) {
	loginPayload, _ := json.Marshal(&model.LoginRequest{
		UID:      uid,
		Password: password,
		Platform: "cli",
	})
	loginMsg, _ := json.Marshal(&model.Message{
		MsgType: model.MsgTypeLogin,
		Content: string(loginPayload),
	})

	fmt.Printf("[client] sending login for %s\n", uid)
	if err := conn.WriteMessage(websocket.TextMessage, loginMsg); err != nil {
		log.Fatalf("login write error: %v", err)
	}
}

func prompt() {
	if targetUID == "" {
		fmt.Print("(no target) > ")
	} else {
		fmt.Printf("[%s] > ", targetUID)
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: client <uid> <password>")
		fmt.Println("  First time: client <uid> <password>  then use /register <password> <nickname>")
		os.Exit(1)
	}

	currentUser = os.Args[1]
	password := os.Args[2]

	addr := "ws://localhost:8080/ws"
	fmt.Printf("Connecting to %s as user %s...\n", addr, currentUser)

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	login(conn, currentUser, password)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
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
						myQQNumber = resp.QQNumber
						fmt.Printf("\r[Server]: login ok, QQ=%d, online=%d\n> ", myQQNumber, resp.Online)
					} else {
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
				fmt.Printf("\r[%s -> %s]: %s\n> ", msg.FromUID, msg.ToUID, msg.Content)

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
					displayFriendList(resp.Friends)
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

			default:
				fmt.Printf("\r[%s]: %s\n> ", msg.FromUID, msg.Content)
				prompt()
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type /help for commands, /to <uid> to start chatting:")
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

		if targetUID == "" {
			fmt.Println("[cmd] no target set, use /to <uid> first")
			prompt()
			continue
		}

		clientSeq++
		sentCount++
		msg := &model.Message{
			ClientSeq: clientSeq,
			MsgType:   model.MsgTypeText,
			FromUID:   currentUser,
			ToUID:     targetUID,
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

func displayFriendList(friends []model.FriendInfo) {
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
				fmt.Printf("    %s QQ:%d  %s  [%s]\n", statusIcon, f.QQNumber, displayName, f.UID)
			}
			displayed[g] = true
		}
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
				fmt.Printf("    %s QQ:%d  %s  [%s]\n", statusIcon, f.QQNumber, displayName, f.UID)
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
			fmt.Printf("  %s QQ:%d  %s  [%s]\n", statusIcon, r.QQNumber, r.Nickname, r.UID)
		}
	}
	fmt.Println("──────────────────────────")
}
