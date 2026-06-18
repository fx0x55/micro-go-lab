package logic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/wokoworks/go-server/common/ecode"
	"github.com/wokoworks/go-server/common/model"
	"github.com/wokoworks/go-server/common/page"
	"github.com/wokoworks/go-server/common/xmetrics"
	"github.com/wokoworks/go-server/service/order/api/internal/svc"
	"github.com/wokoworks/go-server/service/order/api/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// 保留向后兼容的别名
var (
	ErrOrderNotFound           = ecode.ErrOrderNotFound
	ErrUserNotFound            = ecode.ErrUserNotFound
	ErrInvalidStatusTransition = ecode.ErrInvalidStatusTransition
	ErrIdempotencyConflict     = ecode.ErrIdempotencyConflict
)

// idempotencyTTL 是幂等键（占位/响应缓存）的存活窗口。
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
	// 幂等闸门：带 Idempotency-Key 且 Redis 可用时启用。无 key 或无 Redis → 行为不变。
	gateKey := ""
	if idempotencyKey != "" && l.svcCtx.Redis != nil {
		gateKey = orderIDempotencyKey(userID, idempotencyKey)
		acquired, err := l.svcCtx.Redis.SetNX(l.ctx, gateKey, "", idempotencyTTL).Result()
		if err != nil {
			// Redis 故障：降级为不做幂等闸门，不阻断主流程。
			logx.Errorf("idempotency SetNX failed: %v, proceeding without gate", err)
			gateKey = ""
		} else if !acquired {
			// key 已存在：重放（有缓存响应）或并发中（进行中）。
			xmetrics.OrdersCreated.WithLabelValues("conflict").Inc()
			return l.replayOrder(gateKey)
		}
	}

	order, err := l.createOrder(userID, req)
	if err != nil {
		// 创建失败：释放占位，让客户端可用相同 key 重试。
		if gateKey != "" {
			l.svcCtx.Redis.Del(l.ctx, gateKey)
		}
		xmetrics.OrdersCreated.WithLabelValues("error").Inc()
		return nil, err
	}

	// 创建成功：把响应写回占位键，供后续重放返回相同结果。
	if gateKey != "" {
		if b, merr := json.Marshal(order); merr == nil {
			l.svcCtx.Redis.Set(l.ctx, gateKey, b, idempotencyTTL)
		}
	}
	xmetrics.OrdersCreated.WithLabelValues("success").Inc()
	return order, nil
}

// createOrder 执行实际的校验 + 建单（无幂等逻辑）。
func (l *CreateOrderLogic) createOrder(userID uint, req *types.CreateOrderRequest) (*model.Order, error) {
	summary, err := l.svcCtx.UserCli.ValidateUser(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	if !summary.Exists {
		return nil, ErrUserNotFound
	}

	order := &model.Order{
		UserID:      userID,
		ProductName: req.ProductName,
		Amount:      req.Amount,
		Status:      model.StatusPending,
	}
	if err := l.svcCtx.OrderRepo.Create(l.ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// replayOrder 处理幂等键已存在的情形：有缓存响应则返回原结果（重放），
// 否则视为首个请求仍在进行中（并发）。
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

// orderIDempotencyKey 生成幂等占位键：含 userID 防止跨用户碰撞，sha256 限长。
func orderIDempotencyKey(userID uint, key string) string {
	h := sha256.Sum256([]byte(strconv.FormatUint(uint64(userID), 10) + ":" + key))
	return "order:idem:" + hex.EncodeToString(h[:])
}

type GetOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOrderLogic {
	return &GetOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOrderLogic) GetByID(userID, id uint) (*model.Order, error) {
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(l.ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

type ListOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrderLogic {
	return &ListOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListOrderLogic) ListByUserID(userID uint, pg, pageSize int) (*page.Result, error) {
	orders, total, err := l.svcCtx.OrderRepo.FindByUserIDWithPage(l.ctx, userID, (pg-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	return page.NewResult(orders, total, pg, pageSize), nil
}

type UpdateOrderStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateOrderStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateOrderStatusLogic {
	return &UpdateOrderStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateOrderStatusLogic) UpdateStatus(
	userID, id uint,
	req *types.UpdateOrderStatusRequest,
) (*model.Order, error) {
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(l.ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if !isValidTransition(order.Status, req.Status) {
		return nil, ErrInvalidStatusTransition
	}

	order.Status = req.Status
	if err := l.svcCtx.OrderRepo.Update(l.ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}
