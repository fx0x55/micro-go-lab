package logic

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type GetStockLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetStockLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetStockLogic {
	return &GetStockLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetStockLogic) GetStock(in *pb.GetStockRequest) (*pb.GetStockResponse, error) {
	product, err := l.svcCtx.InventoryRepo.GetProduct(l.ctx, in.Sku)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "sku %s not found", in.Sku)
		}
		return nil, status.Errorf(codes.Internal, "query stock failed: %v", err)
	}
	return &pb.GetStockResponse{
		Sku:       product.Sku,
		Total:     int64(product.Total),
		Available: int64(product.Available),
	}, nil
}
