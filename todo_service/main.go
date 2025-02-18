// main.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"config_service/kitex_gen/config"
	"config_service/kitex_gen/config/configservice"
	"github.com/cloudwego/kitex/client"
)

type TodoItem struct {
	ID        int       `json:"id"`
	Content   string    `json:"content" binding:"required"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedBy string    `json:"createdBy" binding:"required"`
	Priority  int       `json:"priority"`
}

var (
	db           *sql.DB
	rdb          *redis.Client
	configClient configservice.Client
)

func main() {
	// 初始化 MySQL
	var err error
	db, err = sql.Open("mysql", "root:0077@tcp(192.168.1.105:3306)/todo_db?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 初始化 Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     "192.168.1.105:6379",
		Password: "",
		DB:       0,
	})

	// 使用 Kitex 工具示例（实际项目需要更完整的集成）
	//utils.NewUUID()
	initConfigClient()

	router := gin.Default()

	router.POST("/todos", createTodo)
	router.GET("/todos/:id", getTodo)
	router.GET("/todos/all", getTodoList)
	router.PUT("/todos/:id", updateTodo)
	router.DELETE("/todos/:id", deleteTodo)

	log.Println("Server started on :10000")

	router.Run(":10000")
}

func initConfigClient() {
	c, err := configservice.NewClient(
		"config",
		client.WithHostPorts("localhost:10001"),
	)
	if err != nil {
		log.Fatal(err)
	}
	configClient = c
}

func createTodo(c *gin.Context) {
	var todo TodoItem
	if err := c.ShouldBindJSON(&todo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	todo.CreatedAt = time.Now()
	todo.UpdatedAt = todo.CreatedAt

	result, err := db.Exec("INSERT INTO todo_items (content, status, createdAt, updatedAt, createdBy, priority) VALUES (?, ?, ?, ?, ?, ?)",
		todo.Content, "pending", todo.CreatedAt, todo.UpdatedAt, todo.CreatedBy, todo.Priority)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	todo.ID = int(id)
	c.JSON(http.StatusCreated, todo)
}

func getTodoList(c *gin.Context) {
	ctx := context.Background()

	// 尝试从 Redis 获取所有 Todo 列表
	cachedTodos, err := rdb.LRange(ctx, "todos", 0, -1).Result()
	if err == nil && len(cachedTodos) > 0 {
		var todos []TodoItem
		for _, cachedTodo := range cachedTodos {
			var todo TodoItem
			if err := json.Unmarshal([]byte(cachedTodo), &todo); err == nil {
				todos = append(todos, todo)
			}
		}
		c.JSON(http.StatusOK, todos)
		return
	}

	// 从数据库查询所有 Todo 项
	rows, err := db.Query("SELECT id, content, status, createdAt, updatedAt, createdBy, priority FROM todo_items")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var todos []TodoItem
	for rows.Next() {
		var todo TodoItem
		if err := rows.Scan(&todo.ID, &todo.Content, &todo.Status, &todo.CreatedAt, &todo.UpdatedAt, &todo.CreatedBy, &todo.Priority); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		todos = append(todos, todo)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 缓存到 Redis
	for _, todo := range todos {
		if jsonData, err := json.Marshal(todo); err == nil {
			rdb.RPush(ctx, "todos", jsonData)
		}
	}
	rdb.Expire(ctx, "todos", 5*time.Minute)

	c.JSON(http.StatusOK, todos)
}

func getTodo(c *gin.Context) {
	id := c.Param("id")
	ctx := context.Background()

	resp, _ := configClient.GetConfig(context.Background(), &config.GetConfigRequest{
		Version: "1.1", // 实际应从请求参数获取
	})

	// 尝试从 Redis 获取
	if resp.UseRedis {
		fmt.Println("using redis config")
		val, err := rdb.Get(ctx, "todo:"+id).Result()
		if err == nil {
			var cachedTodo TodoItem
			if json.Unmarshal([]byte(val), &cachedTodo) == nil {
				c.JSON(http.StatusOK, cachedTodo)
				return
			}
		}
	} else {
		fmt.Println("not using redis config")
	}

	// 从数据库查询
	var todo TodoItem
	row := db.QueryRow("SELECT id, content, status, createdAt, updatedAt, createdBy, priority FROM todo_items WHERE id = ?", id)

	if err := row.Scan(&todo.ID, &todo.Content, &todo.Status, &todo.CreatedAt, &todo.UpdatedAt, &todo.CreatedBy, &todo.Priority); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Todo not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 缓存到 Redis
	if jsonData, err := json.Marshal(todo); err == nil {
		rdb.SetEx(ctx, "todo:"+id, jsonData, 5*time.Minute)
	}

	c.JSON(http.StatusOK, todo)
}

func updateTodo(c *gin.Context) {
	id := c.Param("id")
	var update struct {
		Content  string `json:"content"`
		Status   string `json:"status"`
		Priority int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := db.Exec("UPDATE todo_items SET content = ?, status = ?, priority = ?, updatedAt = ? WHERE id = ?",
		update.Content, update.Status, update.Priority, time.Now(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 清除缓存
	rdb.Del(context.Background(), "todo:"+id)
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func deleteTodo(c *gin.Context) {
	id := c.Param("id")

	if _, err := db.Exec("DELETE FROM todo_items WHERE id = ?", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 清除缓存
	rdb.Del(context.Background(), "todo:"+id)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
