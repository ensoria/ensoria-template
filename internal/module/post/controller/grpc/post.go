package grpc

import (
	"context"

	"github.com/ensoria/ensoria-template/internal/module/post/service"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	pb "github.com/ensoria/ensoria-template/pb/post"
	"google.golang.org/grpc"
)

type PostGRPCService struct {
	pb.UnimplementedPostServer
	dikit.GRPCServiceRegistrar
	service service.PostService
}

func NewPostGRPCService(service service.PostService) *PostGRPCService {
	return &PostGRPCService{
		service: service,
	}
}

// IMPORTANT! gRPCサーバーに登録するためのメソッド
func (s *PostGRPCService) RegisterWithServer(grpcServer *grpc.Server) {
	pb.RegisterPostServer(grpcServer, s)
}

func (s *PostGRPCService) GetPost(ctx context.Context, req *pb.GetPostRequest) (*pb.GetPostResponse, error) {
	return &pb.GetPostResponse{
		PostId:  "1",
		Content: "Sample Post Content",
	}, nil
}
