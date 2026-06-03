package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/qqgo/server/internal/protocol"
	"google.golang.org/protobuf/proto"
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
	historyGroupID        string
	historyGroupName      string
	historyFromTime       string
	historyToTime         string
)

func sendWire(conn *websocket.Conn, wire *pb.WireMessage) {
	data, err := proto.Marshal(wire)
	if err != nil {
		log.Printf("[client] marshal error: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		log.Printf("[client] write error: %v", err)
	}
}

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
		targetGroupID = ""
		historyGroupID = ""
		historyGroupName = ""
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
		if historyGroupID != "" {
			historyOffset += 30
			requestGroupHistory(conn, historyGroupID, historyOffset)
		} else if historyTargetQQ != 0 {
			historyOffset += 30
			requestHistory(conn, historyTargetQQ, historyOffset, historyFromTime, historyToTime)
		} else {
			fmt.Println("[cmd] no chat history context, use /to <qq_number> or /togroup <group_id> first")
		}

	case "/next":
		if historyGroupID != "" {
			if historyOffset >= 30 {
				historyOffset -= 30
			} else {
				historyOffset = 0
			}
			requestGroupHistory(conn, historyGroupID, historyOffset)
		} else if historyTargetQQ != 0 {
			if historyOffset >= 30 {
				historyOffset -= 30
			} else {
				historyOffset = 0
			}
			requestHistory(conn, historyTargetQQ, historyOffset, historyFromTime, historyToTime)
		} else {
			fmt.Println("[cmd] no chat history context, use /to <qq_number> or /togroup <group_id> first")
		}

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
		leaveGroupID := parts[1]
		leaveChatGroup(conn, leaveGroupID)
		if leaveGroupID == targetGroupID {
			targetGroupID = ""
			targetQQ = 0
			historyTargetQQ = 0
			historyGroupID = ""
			historyGroupName = ""
			historyOffset = 0
			fmt.Println("[cmd] left group chat, use /to or /togroup to switch target")
		}

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

	case "/history":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /history <qq> [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
			return true
		}
		targetQQ, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			fmt.Println("[cmd] invalid QQ number")
			return true
		}
		var fromTime, toTime string
		for i := 2; i < len(parts); i++ {
			if parts[i] == "--from" && i+1 < len(parts) {
				fromTime = parts[i+1]
				i++
			} else if parts[i] == "--to" && i+1 < len(parts) {
				toTime = parts[i+1]
				i++
			}
		}
		historyTargetQQ = targetQQ
		historyOffset = 0
		historyFromTime = fromTime
		historyToTime = toTime
		historyGroupID = ""
		requestHistory(conn, targetQQ, 0, fromTime, toTime)

	case "/searchmsg":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /searchmsg <keyword> [qq]")
			return true
		}
		keyword := parts[1]
		var tqq int64
		if len(parts) >= 3 {
			tqq, _ = strconv.ParseInt(parts[2], 10, 64)
		}
		searchMessages(conn, keyword, tqq)

	case "/changepw":
		if len(parts) < 3 {
			fmt.Println("[cmd] Usage: /changepw <old_password> <new_password>")
			return true
		}
		changePassword(conn, parts[1], parts[2])

	case "/block":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /block <qq_number>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		blockUser(conn, qqNum)

	case "/unblock":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /unblock <qq_number>")
			return true
		}
		qqNum, _ := strconv.ParseInt(parts[1], 10, 64)
		unblockUser(conn, qqNum)

	case "/blacklist":
		listBlacklist(conn)

	case "/sendimg":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /sendimg <filepath>")
			return true
		}
		sendFile(conn, parts[1])

	case "/sendfile":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /sendfile <filepath>")
			return true
		}
		sendFile(conn, parts[1])

	case "/recall":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /recall <message_id>")
			return true
		}
		msgID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			fmt.Println("[cmd] invalid message ID")
			return true
		}
		recallMessage(conn, msgID)

	case "/backup":
		requestBackup(conn)

	case "/clean":
		if len(parts) < 2 {
			fmt.Println("[cmd] Usage: /clean <days>")
			return true
		}
		days, err := strconv.Atoi(parts[1])
		if err != nil || days <= 0 {
			fmt.Println("[cmd] invalid days, must be a positive integer")
			return true
		}
		requestClean(conn, days)

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
		fmt.Println("  /changepw <old_password> <new_password> - change password")
		fmt.Println("  /block <qq_number>                    - block a user")
		fmt.Println("  /unblock <qq_number>                  - unblock a user")
		fmt.Println("  /blacklist                            - list blocked users")
		fmt.Println("  /sendimg <filepath>                   - send image")
		fmt.Println("  /sendfile <filepath>                  - send file")
		fmt.Println("  /recall <message_id>                  - recall a message")
		fmt.Println("  /backup                               - backup database")
		fmt.Println("  /clean <days>                         - clean messages older than N days")
		fmt.Println("  /logout                               - logout and clear saved token")
		fmt.Println("  /help                                 - show this help")
		fmt.Println("  /quit                                 - exit")

	case "/quit":
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		time.Sleep(100 * time.Millisecond)
		conn.Close()
		os.Exit(0)

	case "/logout":
		if myQQNumber == 0 {
			fmt.Println("[cmd] not logged in")
			return true
		}
		removeToken(myQQNumber)
		fmt.Printf("[cmd] logged out QQ:%d, tokens cleared\n", myQQNumber)
		myQQNumber = 0
		myNickname = ""
		currentQQ = 0
		targetQQ = 0
		targetGroupID = ""
		historyTargetQQ = 0

	default:
		fmt.Printf("[cmd] unknown command: %s, type /help for help\n", parts[0])
	}
	return true
}

func registerUser(conn *websocket.Conn, password, nickname string) {
	pendingLoginQQ = 0
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_RegisterRequest{RegisterRequest: &pb.RegisterRequest{
		Password: password, Nickname: nickname,
	}}})
	fmt.Printf("[cmd] register request sent for %s\n", nickname)
}

func addFriend(conn *websocket.Conn, qqNumber int64, message string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendRequest{FriendRequest: &pb.FriendRequest{
		ToQqNumber: qqNumber, Message: message,
	}}})
	fmt.Printf("[cmd] friend request sent to qq=%d\n", qqNumber)
}

func acceptFriend(conn *websocket.Conn, qqNumber int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendAccept{FriendAccept: &pb.FriendAccept{ToQqNumber: qqNumber}}})
	fmt.Printf("[cmd] accepting friend request from qq=%d\n", qqNumber)
}

func rejectFriend(conn *websocket.Conn, qqNumber int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendReject{FriendReject: &pb.FriendReject{ToQqNumber: qqNumber}}})
	fmt.Printf("[cmd] rejecting friend request from qq=%d\n", qqNumber)
}

func delFriend(conn *websocket.Conn, qqNumber int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendDelete{FriendDelete: &pb.FriendDelete{ToQqNumber: qqNumber}}})
	fmt.Printf("[cmd] deleting friend qq=%d\n", qqNumber)
}

func listFriends(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendListRequest{FriendListRequest: &pb.FriendListRequest{}}})
}

func searchUser(conn *websocket.Conn, keyword string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendSearchRequest{FriendSearchRequest: &pb.FriendSearchRequest{Keyword: keyword}}})
}

func moveFriend(conn *websocket.Conn, qqNumber int64, groupName string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendMoveGroup{FriendMoveGroup: &pb.FriendMoveGroup{
		QqNumber: qqNumber, GroupName: groupName,
	}}})
	fmt.Printf("[cmd] moving friend qq=%d to group '%s'\n", qqNumber, groupName)
}

func listGroups(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendGroupsRequest{FriendGroupsRequest: &pb.FriendGroupsRequest{}}})
}

func remarkFriend(conn *websocket.Conn, qqNumber int64, remark string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendRemark{FriendRemark: &pb.FriendRemark{
		QqNumber: qqNumber, Remark: remark,
	}}})
	fmt.Printf("[cmd] setting remark for qq=%d: %s\n", qqNumber, remark)
}

func checkUser(conn *websocket.Conn, qq int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_CheckUserRequest{CheckUserRequest: &pb.CheckUserRequest{Qq: qq}}})
}

func requestHistory(conn *websocket.Conn, tqq int64, offset int, fromTime string, toTime string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_HistoryRequest{HistoryRequest: &pb.HistoryRequest{
		TargetQq: tqq, Offset: int32(offset), Limit: 30, FromTime: fromTime, ToTime: toTime,
	}}})
}

func requestGroupHistory(conn *websocket.Conn, groupID string, offset int) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{
		GroupId: groupID, Offset: int32(offset), Limit: 30,
	}}})
}

func searchMessages(conn *websocket.Conn, keyword string, tqq int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_SearchMessagesRequest{SearchMessagesRequest: &pb.SearchMessagesRequest{
		Keyword: keyword, TargetQq: tqq, Limit: 50,
	}}})
}

func createChatGroup(conn *websocket.Conn, name string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: name}}})
}

func joinChatGroup(conn *websocket.Conn, groupID string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: groupID}}})
}

func leaveChatGroup(conn *websocket.Conn, groupID string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupLeaveRequest{GroupLeaveRequest: &pb.GroupLeaveRequest{GroupId: groupID}}})
}

func listMyGroups(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupListRequest{GroupListRequest: &pb.GroupListRequest{}}})
}

func switchToGroup(conn *websocket.Conn, groupID string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_GroupInfoRequest{GroupInfoRequest: &pb.GroupInfoRequest{GroupId: groupID}}})
}

func listSessions(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_SessionListRequest{SessionListRequest: &pb.SessionListRequest{}}})
}

func changePassword(conn *websocket.Conn, oldPw, newPw string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_ChangePasswordRequest{ChangePasswordRequest: &pb.ChangePasswordRequest{
		OldPassword: oldPw, NewPassword: newPw,
	}}})
}

func blockUser(conn *websocket.Conn, qqNumber int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_BlockUserRequest{BlockUserRequest: &pb.BlockUserRequest{Qq: qqNumber}}})
}

func unblockUser(conn *websocket.Conn, qqNumber int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_UnblockUserRequest{UnblockUserRequest: &pb.UnblockUserRequest{Qq: qqNumber}}})
}

func listBlacklist(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_BlacklistRequest{BlacklistRequest: &pb.BlacklistRequest{}}})
}

func sendFile(conn *websocket.Conn, filepath string) {
	if myQQNumber == 0 {
		fmt.Println("[cmd] not logged in")
		return
	}
	if targetQQ == 0 && targetGroupID == "" {
		fmt.Println("[cmd] no target set, use /to or /togroup first")
		return
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Printf("[cmd] read file error: %v\n", err)
		return
	}

	if len(data) > 5*1024*1024 {
		fmt.Println("[cmd] file too large (max 5MB)")
		return
	}

	clientSeq++
	sentCount++
	sendWire(conn, &pb.WireMessage{
		ClientSeq: clientSeq,
		ToQq:      targetQQ,
		GroupId:   targetGroupID,
		Payload: &pb.WireMessage_FileMessage{FileMessage: &pb.FileMessage{
			Filename: filepath, Size: int64(len(data)), Data: data,
		}},
	})
}

func recallMessage(conn *websocket.Conn, messageID int64) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_RecallRequest{RecallRequest: &pb.RecallRequest{MessageId: messageID}}})
}

func createGroup(conn *websocket.Conn, name string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendCreateGroup{FriendCreateGroup: &pb.FriendCreateGroup{Name: name}}})
	fmt.Printf("[cmd] creating group '%s'\n", name)
}

func deleteGroup(conn *websocket.Conn, name string) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendDeleteGroup{FriendDeleteGroup: &pb.FriendDeleteGroup{Name: name}}})
	fmt.Printf("[cmd] deleting group '%s'\n", name)
}

func loginUser(conn *websocket.Conn, qq int64, password string) {
	pendingLoginQQ = qq
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: qq, Password: password, Platform: "cli",
	}}})
	fmt.Printf("[cmd] login request sent for %d\n", qq)
}

func requestBackup(conn *websocket.Conn) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_BackupRequest{BackupRequest: &pb.BackupRequest{}}})
	fmt.Println("[cmd] backup request sent...")
}

func requestClean(conn *websocket.Conn, days int) {
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_CleanRequest{CleanRequest: &pb.CleanRequest{Days: int32(days)}}})
	fmt.Printf("[cmd] clean request sent (messages older than %d days)...\n", days)
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
	initDataDir()

	scheme := os.Getenv("SERVER_SCHEME")
	if scheme == "" {
		scheme = "ws"
	}
	host := os.Getenv("SERVER_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf("%s://%s:%s/ws", scheme, host, port)
	fmt.Printf("Connecting to %s...\n", addr)

	dialer := websocket.DefaultDialer
	if scheme == "wss" {
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	u, _ := url.Parse(addr)
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	fmt.Println("Welcome to QQGO! Use /login or /register to get started.")
	prompt()

	if savedQQ, ok := findSavedQQ(); ok {
		if accessToken, tokOk := loadAccessToken(savedQQ); tokOk {
			fmt.Printf("[cmd] found saved access_token for QQ:%d, auto-login...\n", savedQQ)
			pendingLoginQQ = savedQQ
			sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
				Qq: savedQQ, Token: accessToken, Platform: "cli",
			}}})
		} else if token, tokOk := loadToken(savedQQ); tokOk {
			fmt.Printf("[cmd] found old token for QQ:%d, trying login...\n", savedQQ)
			pendingLoginQQ = savedQQ
			sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
				Qq: savedQQ, Token: token, Platform: "cli",
			}}})
		}
	}

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

			var wire pb.WireMessage
			if err := proto.Unmarshal(data, &wire); err != nil {
				log.Printf("[client] unmarshal error: %v", err)
				continue
			}

			handleWireMessage(conn, &wire)
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

		if targetGroupID != "" {
			appendGroupLog(myQQNumber, targetGroupID, "我", text)
		} else {
			appendPrivateLog(myQQNumber, targetQQ, "我", text)
		}

		sendWire(conn, &pb.WireMessage{
			ClientSeq: clientSeq,
			FromQq:    currentQQ,
			ToQq:      targetQQ,
			GroupId:   targetGroupID,
			Payload:   &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: text}},
		})
		prompt()
	}
}

func handleWireMessage(conn *websocket.Conn, wire *pb.WireMessage) {
	switch p := wire.Payload.(type) {
	case *pb.WireMessage_LoginResponse:
		handleLoginResponse(conn, p.LoginResponse)
	case *pb.WireMessage_RefreshTokenResponse:
		handleRefreshTokenResponse(conn, p.RefreshTokenResponse)
	case *pb.WireMessage_RegisterResponse:
		handleRegisterResponse(p.RegisterResponse)
	case *pb.WireMessage_ServerAck:
		handleServerAck(wire, p.ServerAck)
	case *pb.WireMessage_TextMessage:
		handleTextMessage(conn, wire, p.TextMessage)
	case *pb.WireMessage_FileMessage:
		handleFileMessage(conn, wire, p.FileMessage)
	case *pb.WireMessage_FriendRequest:
		fmt.Printf("\033[2K\r[Friend Request] %s\n> ", p.FriendRequest.Message)
		prompt()
	case *pb.WireMessage_FriendAccept:
		fmt.Printf("\033[2K\r[Friend Accepted] qq=%d\n> ", p.FriendAccept.ToQqNumber)
		prompt()
	case *pb.WireMessage_FriendListResponse:
		displayFriendList(p.FriendListResponse)
		prompt()
	case *pb.WireMessage_FriendSearchResponse:
		displaySearchResults(p.FriendSearchResponse)
		prompt()
	case *pb.WireMessage_FriendGroupsResponse:
		displayFriendGroups(p.FriendGroupsResponse)
		prompt()
	case *pb.WireMessage_CheckUserResponse:
		handleCheckUserResponse(conn, p.CheckUserResponse)
	case *pb.WireMessage_HistoryResponse:
		displayHistory(p.HistoryResponse)
		prompt()
	case *pb.WireMessage_GroupCreateResponse:
		fmt.Printf("\033[2K\r[Group] created: %s (ID: %s)\n> ", p.GroupCreateResponse.Name, p.GroupCreateResponse.GroupId)
		prompt()
	case *pb.WireMessage_GroupListResponse:
		displayGroupList(p.GroupListResponse)
		prompt()
	case *pb.WireMessage_GroupInfoResponse:
		handleGroupInfoResponse(conn, p.GroupInfoResponse)
	case *pb.WireMessage_SessionListResponse:
		displaySessionList(p.SessionListResponse)
		prompt()
	case *pb.WireMessage_GroupHistoryResponse:
		displayGroupHistory(p.GroupHistoryResponse)
		prompt()
	case *pb.WireMessage_SearchMessagesResponse:
		displayMessageSearchResults(p.SearchMessagesResponse)
		prompt()
	case *pb.WireMessage_ChangePasswordResponse:
		handleChangePasswordResponse(p.ChangePasswordResponse)
	case *pb.WireMessage_BlacklistResponse:
		displayBlacklist(p.BlacklistResponse)
		prompt()
	case *pb.WireMessage_RecallNotify:
		fmt.Printf("\033[2K\r[Recall] message #%d recalled by %d\n> ", p.RecallNotify.MessageId, p.RecallNotify.FromQq)
		prompt()
	case *pb.WireMessage_BackupResponse:
		handleBackupResponse(p.BackupResponse)
	case *pb.WireMessage_CleanResponse:
		handleCleanResponse(p.CleanResponse)
	case *pb.WireMessage_Heartbeat:
		// ignore pong
	default:
		fmt.Printf("\033[2K\r[%d]: %v\n> ", wire.FromQq, wire.Payload)
		prompt()
	}
}

func handleLoginResponse(conn *websocket.Conn, resp *pb.LoginResponse) {
	if resp.Code == 0 {
		if pendingLoginQQ != 0 {
			currentQQ = pendingLoginQQ
			pendingLoginQQ = 0
		}
		myQQNumber = resp.QqNumber
		myNickname = resp.Nickname
		if resp.AccessToken != "" {
			saveAccessToken(myQQNumber, resp.AccessToken)
		}
		if resp.RefreshToken != "" {
			saveRefreshToken(myQQNumber, resp.RefreshToken)
		}
		fmt.Printf("\033[2K\r[Server]: login ok, %s(QQ:%d), online=%d\n> ", myNickname, myQQNumber, resp.Online)
	} else {
		if pendingLoginQQ != 0 {
			if resp.Message == "token expired" {
				if refreshToken, ok := loadRefreshToken(pendingLoginQQ); ok {
					fmt.Printf("\033[2K\r[Server]: access token expired, refreshing...\n> ")
					sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_RefreshTokenRequest{RefreshTokenRequest: &pb.RefreshTokenRequest{
						Qq: pendingLoginQQ, RefreshToken: refreshToken,
					}}})
					return
				}
				removeToken(pendingLoginQQ)
				fmt.Printf("\033[2K\r[Server]: refresh token also expired for QQ:%d, please login with password\n> ", pendingLoginQQ)
			} else if resp.Message == "auth failed" {
				removeToken(pendingLoginQQ)
				fmt.Printf("\033[2K\r[Server]: token invalid for QQ:%d, please login with password\n> ", pendingLoginQQ)
			} else {
				fmt.Printf("\033[2K\r[Server]: login failed - %s\n> ", resp.Message)
			}
		} else {
			fmt.Printf("\033[2K\r[Server]: login failed - %s\n> ", resp.Message)
		}
		pendingLoginQQ = 0
	}
}

func handleRefreshTokenResponse(conn *websocket.Conn, resp *pb.RefreshTokenResponse) {
	if resp.Code == 0 {
		saveAccessToken(pendingLoginQQ, resp.AccessToken)
		fmt.Printf("\033[2K\r[Server]: token refreshed, re-login...\n> ")
		sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
			Qq: pendingLoginQQ, Token: resp.AccessToken, Platform: "cli",
		}}})
	} else {
		removeToken(pendingLoginQQ)
		fmt.Printf("\033[2K\r[Server]: refresh failed - %s, please login with password\n> ", resp.Message)
		pendingLoginQQ = 0
	}
}

func handleRegisterResponse(resp *pb.RegisterResponse) {
	if resp.Code == 0 {
		fmt.Printf("\033[2K\r[Server]: register ok, your QQ number is %d\n> ", resp.QqNumber)
	} else {
		fmt.Printf("\033[2K\r[Server]: %s\n> ", resp.Message)
	}
}

func handleServerAck(wire *pb.WireMessage, ack *pb.ServerAck) {
	sentCount--
	if sentCount <= 0 {
		sentCount = 0
		if ack.Content == "not group member" && targetGroupID != "" {
			fmt.Printf("\033[2K\r[sent ✗] %s, leaving group chat window\n> ", ack.Content)
			targetGroupID = ""
			historyGroupID = ""
			historyGroupName = ""
			historyOffset = 0
		} else {
			fmt.Printf("\033[2K\r[sent ✓] %s\n> ", ack.Content)
		}
		prompt()
	}
}

func handleTextMessage(conn *websocket.Conn, wire *pb.WireMessage, text *pb.TextMessage) {
	senderName := fmt.Sprintf("%d", wire.FromQq)
	if wire.FromQq == myQQNumber {
		senderName = "我"
	}
	fmt.Printf("\033[2K\r[%d -> %d]: %s\n> ", wire.FromQq, wire.ToQq, text.Content)

	if wire.GroupId != "" {
		appendGroupLog(myQQNumber, wire.GroupId, senderName, text.Content)
	} else {
		appendPrivateLog(myQQNumber, wire.FromQq, senderName, text.Content)
	}

	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_DeliveredAck{DeliveredAck: &pb.DeliveredAck{MessageId: wire.Id}}})
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_ReadReceipt{ReadReceipt: &pb.ReadReceipt{MessageId: wire.Id}}})
	prompt()
}

type fileContent struct {
	Filename string
	Size     int64
	Data     []byte
}

func saveReceivedFilePB(myQQ int64, fromQQ int64, fc fileContent) string {
	if myQQ == 0 {
		return ""
	}
	ensureUserDir(myQQ)
	recvDir := fmt.Sprintf("DATA/%d/recv", myQQ)
	os.MkdirAll(recvDir, 0755)

	safeName := strconv.FormatInt(fromQQ, 10) + "_" + fc.Filename
	savePath := recvDir + "/" + safeName

	err := os.WriteFile(savePath, fc.Data, 0644)
	if err != nil {
		log.Printf("[localstore] save received file error: %v", err)
		return ""
	}
	return savePath
}

func handleFileMessage(conn *websocket.Conn, wire *pb.WireMessage, file *pb.FileMessage) {
	senderName := fmt.Sprintf("%d", wire.FromQq)
	if wire.FromQq == myQQNumber {
		senderName = "我"
	}

	fmt.Printf("\033[2K\r[%d -> %d]: [File] %s (%d bytes)\n> ", wire.FromQq, wire.ToQq, file.Filename, file.Size)

	fc := fileContent{Filename: file.Filename, Size: file.Size, Data: file.Data}
	savedPath := saveReceivedFilePB(myQQNumber, wire.FromQq, fc)
	if savedPath != "" {
		fmt.Printf("\033[2K\r[Saved to %s]\n> ", savedPath)
	}

	if wire.GroupId != "" {
		appendGroupLog(myQQNumber, wire.GroupId, senderName, fmt.Sprintf("[File] %s (%d bytes)", file.Filename, file.Size))
	} else {
		appendPrivateLog(myQQNumber, wire.FromQq, senderName, fmt.Sprintf("[File] %s (%d bytes)", file.Filename, file.Size))
	}

	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_DeliveredAck{DeliveredAck: &pb.DeliveredAck{MessageId: wire.Id}}})
	sendWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_ReadReceipt{ReadReceipt: &pb.ReadReceipt{MessageId: wire.Id}}})
	prompt()
}

func handleCheckUserResponse(conn *websocket.Conn, resp *pb.CheckUserResponse) {
	if resp.Code == 0 {
		targetQQ = resp.QqNumber
		historyTargetQQ = resp.QqNumber
		historyOffset = 0
		historyFromTime = ""
		historyToTime = ""
		statusIcon := "●"
		if !resp.Online {
			statusIcon = "○"
		}
		fmt.Printf("\033[2K\r[cmd] switched to %s %s(QQ:%d)\n> ", statusIcon, resp.Nickname, resp.QqNumber)
		requestHistory(conn, resp.QqNumber, 0, "", "")
	} else {
		fmt.Printf("\033[2K\r[cmd] %s\n> ", resp.Message)
	}
	prompt()
}

func handleGroupInfoResponse(conn *websocket.Conn, resp *pb.GroupInfoResponse) {
	targetGroupID = resp.GroupId
	targetQQ = 0
	historyTargetQQ = 0
	historyGroupID = resp.GroupId
	historyGroupName = resp.Name
	historyOffset = 0
	fmt.Printf("\033[2K\r[cmd] switched to group: %s (ID: %s, members: %d)\n> ", resp.Name, resp.GroupId, resp.MemberCnt)
	requestGroupHistory(conn, resp.GroupId, 0)
	prompt()
}

func handleChangePasswordResponse(resp *pb.ChangePasswordResponse) {
	if resp.Code == 0 {
		if resp.AccessToken != "" {
			saveAccessToken(myQQNumber, resp.AccessToken)
		}
		if resp.RefreshToken != "" {
			saveRefreshToken(myQQNumber, resp.RefreshToken)
		}
		fmt.Printf("\033[2K\r[Server]: password changed successfully\n> ")
	} else {
		fmt.Printf("\033[2K\r[Server]: %s\n> ", resp.Message)
	}
	prompt()
}

func handleBackupResponse(resp *pb.BackupResponse) {
	if resp.Code == 0 {
		savePath := fmt.Sprintf("DATA/%d/%s", myQQNumber, resp.Filename)
		os.MkdirAll(fmt.Sprintf("DATA/%d", myQQNumber), 0755)
		if err := os.WriteFile(savePath, resp.Data, 0644); err != nil {
			fmt.Printf("\033[2K\r[Backup] save failed: %v\n> ", err)
		} else {
			fmt.Printf("\033[2K\r[Backup] saved to %s (%d bytes)\n> ", savePath, resp.Size)
		}
	} else {
		fmt.Printf("\033[2K\r[Backup] failed: %s\n> ", resp.Message)
	}
	prompt()
}

func handleCleanResponse(resp *pb.CleanResponse) {
	if resp.Code == 0 {
		fmt.Printf("\033[2K\r[Clean] deleted %d messages\n> ", resp.Deleted)
	} else {
		fmt.Printf("\033[2K\r[Clean] failed: %s\n> ", resp.Message)
	}
	prompt()
}

func displayFriendList(resp *pb.FriendListResponse) {
	fmt.Println("\n───── Friend List ─────")

	grouped := make(map[string][]*pb.FriendInfo)
	for _, f := range resp.Friends {
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
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QqNumber, displayName)
			}
			displayed[g] = true
		} else if g == "待处理" {
		} else {
			fmt.Printf("\n  [%s]\n", g)
			displayed[g] = true
		}
	}

	for _, g := range resp.AllGroups {
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
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QqNumber, displayName)
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
				fmt.Printf("    %s QQ:%d  %s\n", statusIcon, f.QqNumber, displayName)
			}
		}
	}

	fmt.Println("──────────────────────")
}

func displaySearchResults(resp *pb.FriendSearchResponse) {
	fmt.Println("\n───── Search Results ─────")
	if len(resp.Results) == 0 {
		fmt.Println("  (no results)")
	} else {
		for _, r := range resp.Results {
			statusIcon := "○"
			if r.Online {
				statusIcon = "●"
			}
			fmt.Printf("  %s QQ:%d  %s\n", statusIcon, r.QqNumber, r.Nickname)
		}
	}
	fmt.Println("──────────────────────────")
}

func displayFriendGroups(resp *pb.FriendGroupsResponse) {
	fmt.Println("\n───── Friend Groups ─────")
	for _, g := range resp.Groups {
		fmt.Printf("  [%s]\n", g)
	}
	fmt.Println("─────────────────────────")
}

func displayHistory(resp *pb.HistoryResponse) {
	if len(resp.Messages) == 0 && resp.Offset == 0 {
		return
	}

	historyTargetNickname = resp.Nickname

	title := fmt.Sprintf("\n───── History with %s (QQ:%d)", resp.Nickname, resp.TargetQq)
	if historyFromTime != "" || historyToTime != "" {
		title += fmt.Sprintf(" %s", func() string {
			f := historyFromTime
			if f == "" {
				f = "..."
			}
			t := historyToTime
			if t == "" {
				t = "..."
			}
			return "[" + f + " ~ " + t + "]"
		}())
	}
	fmt.Println(title + " ─────")
	for _, m := range resp.Messages {
		timeStr := time.Unix(m.CreatedAt, 0).Format("15:04:05")
		if m.FromQq == myQQNumber {
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

func displayGroupList(resp *pb.GroupListResponse) {
	fmt.Println("\n───── My Groups ─────")
	if len(resp.Groups) == 0 {
		fmt.Println("  (no groups)")
	} else {
		for _, g := range resp.Groups {
			ownerMark := ""
			if g.OwnerQq == myQQNumber {
				ownerMark = " [owner]"
			}
			fmt.Printf("  %s  %s (members: %d)%s\n", g.GroupId, g.Name, g.MemberCnt, ownerMark)
		}
	}
	fmt.Println("─────────────────────")
}

func displaySessionList(resp *pb.SessionListResponse) {
	fmt.Println("\n───── Sessions ─────")
	if len(resp.Sessions) == 0 {
		fmt.Println("  (no sessions)")
	} else {
		for _, s := range resp.Sessions {
			if s.Type == "private" {
				statusIcon := "○"
				if s.Online {
					statusIcon = "●"
				}
				timeStr := time.Unix(s.LastTime, 0).Format("01-02 15:04")
				msg := s.LastMessage
				if len(msg) > 30 {
					msg = msg[:30] + "..."
				}
				fmt.Printf("  %s QQ:%-8d  %-12s  %s  %s\n", statusIcon, s.TargetQq, s.Nickname, timeStr, msg)
			} else {
				timeStr := time.Unix(s.LastTime, 0).Format("01-02 15:04")
				msg := s.LastMessage
				if len(msg) > 30 {
					msg = msg[:30] + "..."
				}
				fmt.Printf("  # %-16s  %-12s  %s  %s\n", s.GroupId, s.Nickname, timeStr, msg)
			}
		}
	}
	fmt.Println("────────────────────")
}

func displayGroupHistory(resp *pb.GroupHistoryResponse) {
	if len(resp.Messages) == 0 && resp.Offset == 0 {
		return
	}

	historyGroupName = resp.GroupName

	fmt.Printf("\n───── Group History: %s (%s) ─────\n", resp.GroupName, resp.GroupId)
	for _, m := range resp.Messages {
		timeStr := time.Unix(m.CreatedAt, 0).Format("15:04:05")
		senderName := fmt.Sprintf("%d", m.FromQq)
		if m.FromQq == myQQNumber {
			senderName = "我"
		}
		fmt.Printf("  [%s] %s  %s\n", senderName, timeStr, m.Content)
	}
	if resp.HasMore {
		fmt.Println("  ... (use /prev for older messages)")
	}
	if resp.Offset > 0 {
		fmt.Println("  (use /next for newer messages)")
	}
	fmt.Println("─────────────────────────────────────")
}

func displayMessageSearchResults(resp *pb.SearchMessagesResponse) {
	fmt.Printf("\n───── Search Results: \"%s\" (%d found) ─────\n", resp.Keyword, resp.Total)
	if len(resp.Results) == 0 {
		fmt.Println("  (no results)")
		fmt.Println("─────────────────────────────────────────────")
		return
	}

	for i, item := range resp.Results {
		if i > 0 {
			fmt.Println()
		}

		if item.ContextBefore != nil {
			timeStr := time.Unix(item.ContextBefore.CreatedAt, 0).Format("01-02 15:04")
			sender := fmt.Sprintf("%d", item.ContextBefore.FromQq)
			if item.ContextBefore.FromQq == myQQNumber {
				sender = "我"
			}
			fmt.Printf("    [%s] %s  %s\n", sender, timeStr, item.ContextBefore.Content)
		}

		timeStr := time.Unix(item.CreatedAt, 0).Format("01-02 15:04")
		sender := fmt.Sprintf("%d", item.FromQq)
		if item.FromQq == myQQNumber {
			sender = "我"
		}
		fmt.Printf("  > [%s] %s  %s\n", sender, timeStr, item.Content)

		if item.ContextAfter != nil {
			timeStr := time.Unix(item.ContextAfter.CreatedAt, 0).Format("01-02 15:04")
			sender := fmt.Sprintf("%d", item.ContextAfter.FromQq)
			if item.ContextAfter.FromQq == myQQNumber {
				sender = "我"
			}
			fmt.Printf("    [%s] %s  %s\n", sender, timeStr, item.ContextAfter.Content)
		}
	}
	fmt.Println("─────────────────────────────────────────────")
}

func displayBlacklist(resp *pb.BlacklistResponse) {
	fmt.Println("\n───── Blacklist ─────")
	if len(resp.BlockedUsers) == 0 {
		fmt.Println("  (empty)")
	} else {
		for _, u := range resp.BlockedUsers {
			fmt.Printf("  QQ:%-8d  %s\n", u.QqNumber, u.Nickname)
		}
	}
	fmt.Println("─────────────────────")
}
