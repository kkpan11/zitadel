//go:build integration

package user_test

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/muhlemmer/gu"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zitadel/zitadel/internal/integration"
	object "github.com/zitadel/zitadel/pkg/grpc/object/v2beta"
	user "github.com/zitadel/zitadel/pkg/grpc/user/v2beta"
)

func TestServer_GetUserByID(t *testing.T) {
	orgResp := Tester.CreateOrganization(IamCTX, fmt.Sprintf("GetUserByIDOrg%d", time.Now().UnixNano()), fmt.Sprintf("%d@mouse.com", time.Now().UnixNano()))
	type args struct {
		ctx context.Context
		req *user.GetUserByIDRequest
		dep func(ctx context.Context, username string, request *user.GetUserByIDRequest) error
	}
	tests := []struct {
		name    string
		args    args
		want    *user.GetUserByIDResponse
		wantErr bool
	}{
		{
			name: "user by ID, no id provided",
			args: args{
				IamCTX,
				&user.GetUserByIDRequest{
					Organization: &object.Organization{
						Org: &object.Organization_OrgId{
							OrgId: Tester.Organisation.ID,
						},
					},
					UserId: "",
				},
				func(ctx context.Context, username string, request *user.GetUserByIDRequest) error {
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "user by ID, not found",
			args: args{
				IamCTX,
				&user.GetUserByIDRequest{
					Organization: &object.Organization{
						Org: &object.Organization_OrgId{
							OrgId: Tester.Organisation.ID,
						},
					},
					UserId: "unknown",
				},
				func(ctx context.Context, username string, request *user.GetUserByIDRequest) error {
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "user by ID, ok",
			args: args{
				IamCTX,
				&user.GetUserByIDRequest{
					Organization: &object.Organization{
						Org: &object.Organization_OrgId{
							OrgId: Tester.Organisation.ID,
						},
					},
				},
				func(ctx context.Context, username string, request *user.GetUserByIDRequest) error {
					resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
					request.UserId = resp.GetUserId()
					return nil
				},
			},
			want: &user.GetUserByIDResponse{
				User: &user.User{
					State:              user.UserState_USER_STATE_ACTIVE,
					Username:           "",
					LoginNames:         nil,
					PreferredLoginName: "",
					Type: &user.User_Human{
						Human: &user.HumanUser{
							Profile: &user.HumanProfile{
								GivenName:         "Mickey",
								FamilyName:        "Mouse",
								NickName:          gu.Ptr("Mickey"),
								DisplayName:       gu.Ptr("Mickey Mouse"),
								PreferredLanguage: gu.Ptr("nl"),
								Gender:            user.Gender_GENDER_MALE.Enum(),
								AvatarUrl:         "",
							},
							Email: &user.HumanEmail{
								IsVerified: true,
							},
							Phone: &user.HumanPhone{
								Phone:      "+41791234567",
								IsVerified: true,
							},
						},
					},
				},
				Details: &object.Details{
					ChangeDate:    timestamppb.Now(),
					ResourceOwner: orgResp.OrganizationId,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username := fmt.Sprintf("%d@mouse.com", time.Now().UnixNano())
			err := tt.args.dep(tt.args.ctx, username, tt.args.req)
			require.NoError(t, err)
			retryDuration := time.Minute
			if ctxDeadline, ok := CTX.Deadline(); ok {
				retryDuration = time.Until(ctxDeadline)
			}
			require.EventuallyWithT(t, func(ttt *assert.CollectT) {
				got, getErr := Client.GetUserByID(tt.args.ctx, tt.args.req)
				assertErr := assert.NoError
				if tt.wantErr {
					assertErr = assert.Error
				}
				assertErr(ttt, getErr)
				if getErr != nil {
					return
				}
				tt.want.User.UserId = tt.args.req.GetUserId()
				tt.want.User.Username = username
				tt.want.User.PreferredLoginName = username
				tt.want.User.LoginNames = []string{username}
				if human := tt.want.User.GetHuman(); human != nil {
					human.Email.Email = username
				}
				assert.Equal(ttt, tt.want.User, got.User)
				integration.AssertDetails(t, tt.want, got)
			}, retryDuration, time.Second)
		})
	}
}

type userAttr struct {
	UserID   string
	Username string
}

func TestServer_ListUsers(t *testing.T) {
	orgResp := Tester.CreateOrganization(IamCTX, fmt.Sprintf("ListUsersOrg%d", time.Now().UnixNano()), fmt.Sprintf("%d@mouse.com", time.Now().UnixNano()))
	userResp := Tester.CreateHumanUserVerified(IamCTX, orgResp.OrganizationId, fmt.Sprintf("%d@listusers.com", time.Now().UnixNano()))
	type args struct {
		ctx   context.Context
		count int
		req   *user.ListUsersRequest
		dep   func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error)
	}
	tests := []struct {
		name    string
		args    args
		want    *user.ListUsersResponse
		wantErr bool
	}{
		{
			name: "list user by id, no permission",
			args: args{
				UserCTX,
				0,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					request.Queries = append(request.Queries, InUserIDsQuery([]string{userResp.UserId}))
					return []userAttr{}, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 0,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result:        []*user.User{},
			},
		},
		{
			name: "list user by id, ok",
			args: args{
				IamCTX,
				1,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					infos := make([]userAttr, len(usernames))
					userIDs := make([]string, len(usernames))
					for i, username := range usernames {
						resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
						userIDs[i] = resp.GetUserId()
						infos[i] = userAttr{resp.GetUserId(), username}
					}
					request.Queries = append(request.Queries, InUserIDsQuery(userIDs))
					return infos, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 1,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result: []*user.User{
					{
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "list user by id multiple, ok",
			args: args{
				IamCTX,
				3,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					infos := make([]userAttr, len(usernames))
					userIDs := make([]string, len(usernames))
					for i, username := range usernames {
						resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
						userIDs[i] = resp.GetUserId()
						infos[i] = userAttr{resp.GetUserId(), username}
					}
					request.Queries = append(request.Queries, InUserIDsQuery(userIDs))
					return infos, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 3,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result: []*user.User{
					{
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					}, {
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					}, {
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "list user by username, ok",
			args: args{
				IamCTX,
				1,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					infos := make([]userAttr, len(usernames))
					userIDs := make([]string, len(usernames))
					for i, username := range usernames {
						resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
						userIDs[i] = resp.GetUserId()
						infos[i] = userAttr{resp.GetUserId(), username}
						request.Queries = append(request.Queries, UsernameQuery(username))
					}
					return infos, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 1,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result: []*user.User{
					{
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "list user in emails, ok",
			args: args{
				IamCTX,
				1,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					infos := make([]userAttr, len(usernames))
					for i, username := range usernames {
						resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
						infos[i] = userAttr{resp.GetUserId(), username}
					}
					request.Queries = append(request.Queries, InUserEmailsQuery(usernames))
					return infos, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 1,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result: []*user.User{
					{
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "list user in emails multiple, ok",
			args: args{
				IamCTX,
				3,
				&user.ListUsersRequest{},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					infos := make([]userAttr, len(usernames))
					for i, username := range usernames {
						resp := Tester.CreateHumanUserVerified(ctx, orgResp.OrganizationId, username)
						infos[i] = userAttr{resp.GetUserId(), username}
					}
					request.Queries = append(request.Queries, InUserEmailsQuery(usernames))
					return infos, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 3,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result: []*user.User{
					{
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					}, {
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					}, {
						State: user.UserState_USER_STATE_ACTIVE,
						Type: &user.User_Human{
							Human: &user.HumanUser{
								Profile: &user.HumanProfile{
									GivenName:         "Mickey",
									FamilyName:        "Mouse",
									NickName:          gu.Ptr("Mickey"),
									DisplayName:       gu.Ptr("Mickey Mouse"),
									PreferredLanguage: gu.Ptr("nl"),
									Gender:            user.Gender_GENDER_MALE.Enum(),
								},
								Email: &user.HumanEmail{
									IsVerified: true,
								},
								Phone: &user.HumanPhone{
									Phone:      "+41791234567",
									IsVerified: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "list user in emails no found, ok",
			args: args{
				IamCTX,
				3,
				&user.ListUsersRequest{Queries: []*user.SearchQuery{
					InUserEmailsQuery([]string{"notfound"}),
				},
				},
				func(ctx context.Context, org string, usernames []string, request *user.ListUsersRequest) ([]userAttr, error) {
					return []userAttr{}, nil
				},
			},
			want: &user.ListUsersResponse{
				Details: &object.ListDetails{
					TotalResult: 0,
					Timestamp:   timestamppb.Now(),
				},
				SortingColumn: 0,
				Result:        []*user.User{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usernames := make([]string, tt.args.count)
			for i := 0; i < tt.args.count; i++ {
				usernames[i] = fmt.Sprintf("%d%d@mouse.com", time.Now().UnixNano(), i)
			}
			infos, err := tt.args.dep(tt.args.ctx, orgResp.OrganizationId, usernames, tt.args.req)
			require.NoError(t, err)
			retryDuration := time.Minute
			if ctxDeadline, ok := CTX.Deadline(); ok {
				retryDuration = time.Until(ctxDeadline)
			}
			require.EventuallyWithT(t, func(ttt *assert.CollectT) {
				got, listErr := Client.ListUsers(tt.args.ctx, tt.args.req)
				assertErr := assert.NoError
				if tt.wantErr {
					assertErr = assert.Error
				}
				assertErr(ttt, listErr)
				if listErr != nil {
					return
				}
				// always only give back dependency infos which are required for the response
				assert.Len(ttt, tt.want.Result, len(infos))
				// always first check length, otherwise its failed anyway
				assert.Len(ttt, got.Result, len(tt.want.Result))
				// fill in userid and username as it is generated
				for i := range infos {
					tt.want.Result[i].UserId = infos[i].UserID
					tt.want.Result[i].Username = infos[i].Username
					tt.want.Result[i].PreferredLoginName = infos[i].Username
					tt.want.Result[i].LoginNames = []string{infos[i].Username}
					if human := tt.want.Result[i].GetHuman(); human != nil {
						human.Email.Email = infos[i].Username
					}
				}
				for i := range tt.want.Result {
					assert.Contains(ttt, got.Result, tt.want.Result[i])
				}
				integration.AssertListDetails(t, tt.want, got)
			}, retryDuration, time.Millisecond*100, "timeout waiting for expected user result")
		})
	}
}

func InUserIDsQuery(ids []string) *user.SearchQuery {
	return &user.SearchQuery{Query: &user.SearchQuery_InUserIdsQuery{
		InUserIdsQuery: &user.InUserIDQuery{
			UserIds: ids,
		},
	},
	}
}

func InUserEmailsQuery(emails []string) *user.SearchQuery {
	return &user.SearchQuery{Query: &user.SearchQuery_InUserEmailsQuery{
		InUserEmailsQuery: &user.InUserEmailsQuery{
			UserEmails: emails,
		},
	},
	}
}

func UsernameQuery(username string) *user.SearchQuery {
	return &user.SearchQuery{Query: &user.SearchQuery_UserNameQuery{
		UserNameQuery: &user.UserNameQuery{
			UserName: username,
		},
	},
	}
}
