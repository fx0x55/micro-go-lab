package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound        = ecode.ErrUserNotFound
	ErrIdempotencyConflict = ecode.ErrIdempotencyConflict
)

const idempotencyTTL = 15 * time.Minute

var validTransitions = map[string]map[string]bool{
	model.StatusPending:   {model.StatusPaid: true, model.StatusCancelled: true},
	model.StatusPaid:      {},
	model.StatusCancelled: {},
}

type CreateOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrderLogic {
	return &CreateOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOrderLogic) Create(
	userID uint,
	req *types.CreateOrderRequest,
	idempotencyKey string,
) (*model.Order, error) {
	// troubleshooting lab: BUG_CPU=1 时在创建订单热路径上触发 CPU 热点故障。
	// 放在所有副作用之前，使得任何持合法 JWT 的请求都会烧 CPU（即便用户在库里不存在），
	// 降低实验触发门槛。
	if bugEnabled("BUG_CPU") {
		computeRiskScore(req)
	}

	// troubleshooting lab: BUG_MEMLEAK=1 时把"风控画像"塞进全局缓存，永不淘汰——
	// 模拟线上最常见的内存泄漏（缓存无 TTL 无上限）。放在副作用之前，便于压测复现。
	if bugEnabled("BUG_MEMLEAK") {
		cacheRiskProfile(userID, req, idempotencyKey)
	}

	gateKey := ""
	if idempotencyKey != "" && l.svcCtx.Redis != nil {
		gateKey = orderIDempotencyKey(userID, idempotencyKey)
		acquired, err := l.svcCtx.Redis.SetNX(l.ctx, gateKey, "", idempotencyTTL).Result()
		if err != nil {
			logx.Errorf("idempotency SetNX failed: %v, proceeding without gate", err)
			gateKey = ""
		} else if !acquired {
			xmetrics.OrdersCreated.WithLabelValues("conflict").Inc()
			return l.replayOrder(gateKey)
		}
	}

	order, err := l.createOrder(userID, req)
	if err != nil {
		if gateKey != "" {
			l.svcCtx.Redis.Del(l.ctx, gateKey)
		}
		xmetrics.OrdersCreated.WithLabelValues("error").Inc()
		return nil, err
	}

	if gateKey != "" {
		if b, merr := json.Marshal(order); merr == nil {
			l.svcCtx.Redis.Set(l.ctx, gateKey, b, idempotencyTTL)
		}
	}
	xmetrics.OrdersCreated.WithLabelValues("success").Inc()
	return order, nil
}

func (l *CreateOrderLogic) createOrder(userID uint, req *types.CreateOrderRequest) (*model.Order, error) {
	summary, err := l.svcCtx.UserCli.ValidateUser(l.ctx, &pb.ValidateUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		return nil, err
	}
	if !summary.Exists {
		return nil, ErrUserNotFound
	}

	order := &model.Order{
		UserID:      userID,
		Sku:         req.Sku,
		Quantity:    req.Quantity,
		ProductName: req.ProductName,
		Amount:      req.Amount,
		Status:      model.StatusPending,
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if err := l.svcCtx.OrderRepo.Create(tx, order); err != nil {
			return err
		}

		payload, err := json.Marshal(xevent.Envelope{
			Event:      xevent.OrderCreated,
			Version:    1,
			OccurredAt: time.Now(),
			Data: xevent.OrderCreatedData{
				OrderID:     order.ID,
				UserID:      order.UserID,
				Sku:         order.Sku,
				Quantity:    order.Quantity,
				ProductName: order.ProductName,
				Amount:      order.Amount,
				Status:      order.Status,
			},
		})
		if err != nil {
			return err
		}
		outboxEvent := &xevent.OutboxEvent{
			EventID:   uuid.New().String(),
			Topic:     xevent.TopicOrderEvents,
			EventKey:  strconv.FormatUint(uint64(order.UserID), 10),
			EventType: string(xevent.OrderCreated),
			Version:   1,
			Payload:   string(payload),
			Status:    xevent.OutboxStatusPending,
		}
		return l.svcCtx.OutboxRepo.Insert(tx, outboxEvent)
	})
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (l *CreateOrderLogic) replayOrder(gateKey string) (*model.Order, error) {
	val, err := l.svcCtx.Redis.Get(l.ctx, gateKey).Bytes()
	if err == nil && len(val) > 0 {
		var cached model.Order
		if jerr := json.Unmarshal(val, &cached); jerr == nil {
			return &cached, nil
		}
	}
	return nil, ErrIdempotencyConflict
}

func orderIDempotencyKey(userID uint, key string) string {
	h := sha256.Sum256([]byte(strconv.FormatUint(uint64(userID), 10) + ":" + key))
	return "order:idem:" + hex.EncodeToString(h[:])
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}
