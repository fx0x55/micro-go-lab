package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/model"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type CreateUserLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserLogic {
	return &CreateUserLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateUser 创建用户：唯一性校验 + bcrypt 哈希 + 写 user + 写 outbox（同一事务）。
// 密码明文仅在此方法内存活，哈希永不出 user-rpc。
func (l *CreateUserLogic) CreateUser(in *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// 唯一性预检：用户名/邮箱已存在 → gRPC AlreadyExists 语义。
	if _, err := l.svcCtx.UserRepo.FindByUsername(l.ctx, in.Username); err == nil {
		return nil, status.Error(codes.AlreadyExists, "username already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, status.Errorf(codes.Internal, "query username failed: %v", err)
	}
	if _, err := l.svcCtx.UserRepo.FindByEmail(l.ctx, in.Email); err == nil {
		return nil, status.Error(codes.AlreadyExists, "email already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, status.Errorf(codes.Internal, "query email failed: %v", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash password failed: %v", err)
	}

	user := &model.User{
		Username: in.Username,
		Password: string(hashed),
		Email:    in.Email,
	}

	// 写 user + outbox 事件必须在同一事务，保证「数据写入」与「事件发布」原子。
	tx := l.svcCtx.DB.Begin()
	if tx.Error != nil {
		return nil, status.Error(codes.Internal, "begin tx failed")
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := l.svcCtx.UserRepo.Create(tx, user); err != nil {
		tx.Rollback()
		// 并发下两个请求同时通过预检 → 唯一索引兜底，映射为 AlreadyExists。
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			return nil, status.Error(codes.AlreadyExists, "username or email already exists")
		}
		return nil, status.Errorf(codes.Internal, "create user failed: %v", err)
	}

	// 写 outbox 事件（同一事务），UserRegistered 由 Poller 异步发往 Redis Stream。
	payload, _ := json.Marshal(xevent.Envelope{
		Event:      xevent.UserRegistered,
		Version:    1,
		OccurredAt: time.Now(),
		Data: xevent.UserRegisteredData{
			UserID:   user.ID,
			Username: user.Username,
		},
	})
	outboxEvent := &xevent.OutboxEvent{
		EventID:   uuid.New().String(),
		Topic:     xevent.TopicUserEvents,
		EventKey:  strconv.FormatUint(uint64(user.ID), 10),
		EventType: string(xevent.UserRegistered),
		Version:   1,
		Payload:   string(payload),
		Status:    xevent.OutboxStatusPending,
	}
	if err := l.svcCtx.OutboxRepo.Insert(tx, outboxEvent); err != nil {
		tx.Rollback()
		return nil, status.Error(codes.Internal, "write outbox failed")
	}

	if err := tx.Commit().Error; err != nil {
		return nil, status.Error(codes.Internal, "commit tx failed")
	}

	// 注册后用户资料变更会影响 ValidateUser 缓存；新用户尚无缓存，无需失效。
	return &pb.CreateUserResponse{
		Id:       uint64(user.ID),
		Username: user.Username,
		Email:    user.Email,
	}, nil
}
