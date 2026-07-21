package repository

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/common/xlab"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InventoryRepositoryInterface 定义库存数据访问层的接口。
type InventoryRepositoryInterface interface {
	GetProduct(ctx context.Context, sku string) (*model.Product, error)
	// Reserve 在给定事务内预占库存：原子扣减 available + 插 reservation。
	// available 不足时扣减 affected rows=0，调用方据此判断缺货。
	Reserve(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error)
	// FindReservationByOrderID 查找订单对应的预占记录。
	FindReservationByOrderID(ctx context.Context, orderID uint) (*model.Reservation, error)
	// Release 在给定事务内释放预占：翻 reservation status->released + 回补 available。
	// 顺序：先翻 status，再回补 available（顺序不可反）。
	Release(tx *gorm.DB, orderID uint) (int64, error)
}

type InventoryRepository struct {
	db *gorm.DB
}

func NewInventoryRepository(db *gorm.DB) *InventoryRepository {
	return &InventoryRepository{db: db}
}

func (r *InventoryRepository) GetProduct(ctx context.Context, sku string) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).Where("sku = ?", sku).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *InventoryRepository) Reserve(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error) {
	// BUG_DB_DEADLOCK=1：回退到"先 product 后 reservation"的反序加锁，用于死锁 lab 展示脆弱写法。
	// 默认（0）走 reserveOrdered，与 Release 统一为 reservation->product，消除 AB-BA。
	if xlab.BugEnabled("BUG_DB_DEADLOCK") {
		return r.reserveReversed(tx, orderID, sku, qty)
	}
	return r.reserveOrdered(tx, orderID, sku, qty)
}

// reserveOrdered 是默认的正确写法：先锁 reservation 位，再扣 product，最后写 reservation。
// 与 Release 的"先读 reservation(FOR UPDATE) 再改 product"顺序一致，两者不会构成加锁环。
func (r *InventoryRepository) reserveOrdered(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error) {
	// 1. 先锁 reservation 位（order_id 唯一索引；即便无记录也加 gap 锁，锁顺序被固定）。
	var existing model.Reservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", orderID).First(&existing).Error; err != nil &&
		!errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	// 2. 原子扣减 available（不足时 RowsAffected=0）。
	if affected := decrementAvailable(tx, sku, qty); affected == 0 {
		return 0, nil
	} else if affected < 0 {
		return 0, decrementErr(tx)
	}
	// 3. 写 reservation 记录。
	if err := tx.Create(&model.Reservation{
		OrderID:  orderID,
		Sku:      sku,
		Quantity: qty,
		Status:   model.ReservationStatusReserved,
	}).Error; err != nil {
		return 0, err
	}
	return 1, nil
}

// reserveReversed 是 BUG_DB_DEADLOCK=1 时的脆弱写法：先扣 product（拿 product 锁），
// 再 sleep 撑大窗口，最后写 reservation（拿 reservation 锁）——与 Release 的
// "reservation->product"构成 AB-BA。真实生产没有 sleep，但锁顺序反了高并发下照样爆。
func (r *InventoryRepository) reserveReversed(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error) {
	if affected := decrementAvailable(tx, sku, qty); affected == 0 {
		return 0, nil
	} else if affected < 0 {
		return 0, decrementErr(tx)
	}
	r.deadlockSleep(tx)
	if err := tx.Create(&model.Reservation{
		OrderID:  orderID,
		Sku:      sku,
		Quantity: qty,
		Status:   model.ReservationStatusReserved,
	}).Error; err != nil {
		return 0, err
	}
	return 1, nil
}

func (r *InventoryRepository) FindReservationByOrderID(
	ctx context.Context, orderID uint,
) (*model.Reservation, error) {
	var res model.Reservation
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&res).Error
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Release 先翻 reservation status，再回补 available。
// 仅对 status=reserved 的记录生效；已 released 则 affected=0（no-op，幂等）。
func (r *InventoryRepository) Release(tx *gorm.DB, orderID uint) (int64, error) {
	var res model.Reservation
	// FOR UPDATE：锁定即将修改的行，既避免释放并发下的 lost update，也固定 reservation 锁顺序。
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", orderID).First(&res).Error; err != nil {
		return 0, err
	}
	if res.Status == model.ReservationStatusReleased {
		return 0, nil
	}
	if err := tx.Model(&model.Reservation{}).
		Where("order_id = ? AND status = ?", orderID, model.ReservationStatusReserved).
		Update("status", model.ReservationStatusReleased).Error; err != nil {
		return 0, err
	}
	// BUG_DB_DEADLOCK=1：翻完 status（持有 reservation 锁）后撑大窗口，再去拿 product 锁，
	// 与 reserveReversed 的"product->reservation"构成 AB-BA，便于死锁 lab 复现。
	if xlab.BugEnabled("BUG_DB_DEADLOCK") {
		r.deadlockSleep(tx)
	}
	if err := tx.Model(&model.Product{}).
		Where("sku = ?", res.Sku).
		Update("available", gorm.Expr("available + ?", res.Quantity)).Error; err != nil {
		return 0, err
	}
	return 1, nil
}

// decrementAvailable 原子扣减库存：available 充足时扣减并返回 1；不足返回 0；出错返回 -1。
func decrementAvailable(tx *gorm.DB, sku string, qty int) int64 {
	result := tx.Model(&model.Product{}).
		Where("sku = ? AND available >= ?", sku, qty).
		Update("available", gorm.Expr("available - ?", qty))
	if result.Error != nil {
		return -1
	}
	return result.RowsAffected
}

// decrementErr 取出 decrementAvailable 留在事务上的错误。
// 之所以这样拆，是因为 reserveOrdered/reserveReversed 都需要在"扣减失败"时区分
// "缺货（affected=0，正常）"与"出错（affected<0，需返回 err）"两条分支。
func decrementErr(tx *gorm.DB) error {
	return tx.Error
}

// deadlockSleep 在事务内用 SELECT SLEEP(?) 撑大竞争窗口（仅 BUG_DB_DEADLOCK=1 时调用）。
// 用 DB 侧 sleep 而非 time.Sleep，是为了让事务期间持有的锁一直保留，确保死锁窗口存在。
// 时长由 BUG_DEADLOCK_SLEEP_MS 控制，默认 300ms。
func (r *InventoryRepository) deadlockSleep(tx *gorm.DB) {
	ms := xlab.BugEnvInt("BUG_DEADLOCK_SLEEP_MS", 300)
	if ms <= 0 {
		return
	}
	_ = tx.Exec("SELECT SLEEP(?)", float64(ms)/1000.0).Error
}
