import os

def create_directory(path):
    os.makedirs(path, exist_ok=True)

def create_file(path, content=""):
    with open(path, 'w') as f:
        f.write(content)

def create_project_structure():
    # Root directory
    create_directory("project-root")
    os.chdir("project-root")

    # App directory and its subdirectories
    create_directory("app/api/client/auth/register")
    create_file("app/api/client/auth/register/route.ts")
    create_directory("app/api/client/auth/verify-otp")
    create_file("app/api/client/auth/verify-otp/route.ts")
    create_directory("app/api/client/auth/login")
    create_file("app/api/client/auth/login/route.ts")
    create_directory("app/api/client/auth/verify-login-otp")
    create_file("app/api/client/auth/verify-login-otp/route.ts")
    create_directory("app/api/client/auth/refresh-token")
    create_file("app/api/client/auth/refresh-token/route.ts")
    create_directory("app/api/client/auth/logout")
    create_file("app/api/client/auth/logout/route.ts")
    create_directory("app/api/client/auth/qr-login/generate")
    create_file("app/api/client/auth/qr-login/generate/route.ts")
    create_directory("app/api/client/auth/qr-login/verify")
    create_file("app/api/client/auth/qr-login/verify/route.ts")
    create_directory("app/api/client/auth/qr-login/status")
    create_file("app/api/client/auth/qr-login/status/route.ts")
    create_directory("app/api/client/auth/resend-otp")
    create_file("app/api/client/auth/resend-otp/route.ts")
    
    create_directory("app/api/client/user/profile")
    create_file("app/api/client/user/profile/route.ts")
    create_directory("app/api/client/user/avatar")
    create_file("app/api/client/user/avatar/route.ts")
    create_directory("app/api/client/user/status")
    create_file("app/api/client/user/status/route.ts")
    create_directory("app/api/client/user/search")
    create_file("app/api/client/user/search/route.ts")
    create_directory("app/api/client/user/contacts/block")
    create_file("app/api/client/user/contacts/block/route.ts")
    create_directory("app/api/client/user/contacts/blocked")
    create_file("app/api/client/user/contacts/blocked/route.ts")
    create_directory("app/api/client/user/contacts")
    create_file("app/api/client/user/contacts/route.ts")
    create_directory("app/api/client/user/online-status")
    create_file("app/api/client/user/online-status/route.ts")
    
    create_directory("app/api/client/privacy/settings")
    create_file("app/api/client/privacy/settings/route.ts")
    create_directory("app/api/client/privacy/last-seen")
    create_file("app/api/client/privacy/last-seen/route.ts")
    create_directory("app/api/client/privacy/profile-photo")
    create_file("app/api/client/privacy/profile-photo/route.ts")
    create_directory("app/api/client/privacy/status")
    create_file("app/api/client/privacy/status/route.ts")
    create_directory("app/api/client/privacy/read-receipts")
    create_file("app/api/client/privacy/read-receipts/route.ts")
    
    create_directory("app/api/client/chats/[chatId]/messages/[messageId]/read")
    create_file("app/api/client/chats/[chatId]/messages/[messageId]/read/route.ts")
    create_directory("app/api/client/chats/[chatId]/messages/[messageId]/reactions")
    create_file("app/api/client/chats/[chatId]/messages/[messageId]/reactions/route.ts")
    create_directory("app/api/client/chats/[chatId]/messages/unread-count")
    create_file("app/api/client/chats/[chatId]/messages/unread-count/route.ts")
    create_directory("app/api/client/chats/[chatId]/messages/search")
    create_file("app/api/client/chats/[chatId]/messages/search/route.ts")
    create_directory("app/api/client/chats/[chatId]/messages")
    create_file("app/api/client/chats/[chatId]/messages/route.ts")
    create_directory("app/api/client/chats/[chatId]/typing")
    create_file("app/api/client/chats/[chatId]/typing/route.ts")
    create_directory("app/api/client/chats/[chatId]/mute")
    create_file("app/api/client/chats/[chatId]/mute/route.ts")
    create_directory("app/api/client/chats/[chatId]/clear")
    create_file("app/api/client/chats/[chatId]/clear/route.ts")
    create_directory("app/api/client/chats/[chatId]")
    create_file("app/api/client/chats/[chatId]/route.ts")
    create_directory("app/api/client/chats/create")
    create_file("app/api/client/chats/create/route.ts")
    create_directory("app/api/client/chats")
    create_file("app/api/client/chats/route.ts")
    
    create_directory("app/api/client/groups/[groupId]/members/[userId]")
    create_file("app/api/client/groups/[groupId]/members/[userId]/route.ts")
    create_directory("app/api/client/groups/[groupId]/members/promote")
    create_file("app/api/client/groups/[groupId]/members/promote/route.ts")
    create_directory("app/api/client/groups/[groupId]/members/demote")
    create_file("app/api/client/groups/[groupId]/members/demote/route.ts")
    create_directory("app/api/client/groups/[groupId]/members")
    create_file("app/api/client/groups/[groupId]/members/route.ts")
    create_directory("app/api/client/groups/[groupId]/leave")
    create_file("app/api/client/groups/[groupId]/leave/route.ts")
    create_directory("app/api/client/groups/[groupId]/avatar")
    create_file("app/api/client/groups/[groupId]/avatar/route.ts")
    create_directory("app/api/client/groups/[groupId]/settings")
    create_file("app/api/client/groups/[groupId]/settings/route.ts")
    create_directory("app/api/client/groups/[groupId]")
    create_file("app/api/client/groups/[groupId]/route.ts")
    create_directory("app/api/client/groups/invite/generate")
    create_file("app/api/client/groups/invite/generate/route.ts")
    create_directory("app/api/client/groups/invite/join")
    create_file("app/api/client/groups/invite/join/route.ts")
    create_directory("app/api/client/groups")
    create_file("app/api/client/groups/route.ts")
    
    create_directory("app/api/client/media/upload/image")
    create_file("app/api/client/media/upload/image/route.ts")
    create_directory("app/api/client/media/upload/video")
    create_file("app/api/client/media/upload/video/route.ts")
    create_directory("app/api/client/media/upload/audio")
    create_file("app/api/client/media/upload/audio/route.ts")
    create_directory("app/api/client/media/upload/document")
    create_file("app/api/client/media/upload/document/route.ts")
    create_directory("app/api/client/media/upload/voice-note")
    create_file("app/api/client/media/upload/voice-note/route.ts")
    create_directory("app/api/client/media/download/[fileId]")
    create_file("app/api/client/media/download/[fileId]/route.ts")
    create_directory("app/api/client/media/thumbnail/[fileId]")
    create_file("app/api/client/media/thumbnail/[fileId]/route.ts")
    create_directory("app/api/client/media/delete/[fileId]")
    create_file("app/api/client/media/delete/[fileId]/route.ts")
    
    create_directory("app/api/client/calls/initiate")
    create_file("app/api/client/calls/initiate/route.ts")
    create_directory("app/api/client/calls/answer")
    create_file("app/api/client/calls/answer/route.ts")
    create_directory("app/api/client/calls/reject")
    create_file("app/api/client/calls/reject/route.ts")
    create_directory("app/api/client/calls/end")
    create_file("app/api/client/calls/end/route.ts")
    create_directory("app/api/client/calls/[callId]/ice-candidates")
    create_file("app/api/client/calls/[callId]/ice-candidates/route.ts")
    create_directory("app/api/client/calls/[callId]/offer")
    create_file("app/api/client/calls/[callId]/offer/route.ts")
    create_directory("app/api/client/calls/[callId]/answer-sdp")
    create_file("app/api/client/calls/[callId]/answer-sdp/route.ts")
    create_directory("app/api/client/calls/[callId]")
    create_file("app/api/client/calls/[callId]/route.ts")
    create_directory("app/api/client/calls/turn-credentials")
    create_file("app/api/client/calls/turn-credentials/route.ts")
    create_directory("app/api/client/calls/group-call/create")
    create_file("app/api/client/calls/group-call/create/route.ts")
    create_directory("app/api/client/calls/group-call/join")
    create_file("app/api/client/calls/group-call/join/route.ts")
    create_directory("app/api/client/calls")
    create_file("app/api/client/calls/route.ts")
    
    create_directory("app/api/client/status/[statusId]/view")
    create_file("app/api/client/status/[statusId]/view/route.ts")
    create_directory("app/api/client/status/[statusId]/viewers")
    create_file("app/api/client/status/[statusId]/viewers/route.ts")
    create_directory("app/api/client/status/[statusId]")
    create_file("app/api/client/status/[statusId]/route.ts")
    create_directory("app/api/client/status/my-status")
    create_file("app/api/client/status/my-status/route.ts")
    create_directory("app/api/client/status/recent")
    create_file("app/api/client/status/recent/route.ts")
    create_directory("app/api/client/status")
    create_file("app/api/client/status/route.ts")
    
    create_directory("app/api/client/notifications/read")
    create_file("app/api/client/notifications/read/route.ts")
    create_directory("app/api/client/notifications/settings")
    create_file("app/api/client/notifications/settings/route.ts")
    create_directory("app/api/client/notifications/push-token")
    create_file("app/api/client/notifications/push-token/route.ts")
    create_directory("app/api/client/notifications")
    create_file("app/api/client/notifications/route.ts")
    
    create_directory("app/api/client/sync/messages")
    create_file("app/api/client/sync/messages/route.ts")
    create_directory("app/api/client/sync/chats")
    create_file("app/api/client/sync/chats/route.ts")
    create_directory("app/api/client/sync/full")
    create_file("app/api/client/sync/full/route.ts")
    
    create_directory("app/api/admin/auth/login")
    create_file("app/api/admin/auth/login/route.ts")
    create_directory("app/api/admin/auth/logout")
    create_file("app/api/admin/auth/logout/route.ts")
    create_directory("app/api/admin/auth/verify")
    create_file("app/api/admin/auth/verify/route.ts")
    
    create_directory("app/api/admin/dashboard/stats")
    create_file("app/api/admin/dashboard/stats/route.ts")
    create_directory("app/api/admin/dashboard/analytics")
    create_file("app/api/admin/dashboard/analytics/route.ts")
    create_directory("app/api/admin/dashboard/system-health")
    create_file("app/api/admin/dashboard/system-health/route.ts")
    
    create_directory("app/api/admin/users/[userId]/ban")
    create_file("app/api/admin/users/[userId]/ban/route.ts")
    create_directory("app/api/admin/users/[userId]/unban")
    create_file("app/api/admin/users/[userId]/unban/route.ts")
    create_directory("app/api/admin/users/[userId]/chats")
    create_file("app/api/admin/users/[userId]/chats/route.ts")
    create_directory("app/api/admin/users/[userId]/activity")
    create_file("app/api/admin/users/[userId]/activity/route.ts")
    create_directory("app/api/admin/users/[userId]")
    create_file("app/api/admin/users/[userId]/route.ts")
    create_directory("app/api/admin/users/search")
    create_file("app/api/admin/users/search/route.ts")
    create_directory("app/api/admin/users/bulk-actions")
    create_file("app/api/admin/users/bulk-actions/route.ts")
    create_directory("app/api/admin/users/export")
    create_file("app/api/admin/users/export/route.ts")
    create_directory("app/api/admin/users")
    create_file("app/api/admin/users/route.ts")
    
    create_directory("app/api/admin/messages/[messageId]/flag")
    create_file("app/api/admin/messages/[messageId]/flag/route.ts")
    create_directory("app/api/admin/messages/[messageId]")
    create_file("app/api/admin/messages/[messageId]/route.ts")
    create_directory("app/api/admin/messages/reported")
    create_file("app/api/admin/messages/reported/route.ts")
    create_directory("app/api/admin/messages/search")
    create_file("app/api/admin/messages/search/route.ts")
    create_directory("app/api/admin/messages/analytics")
    create_file("app/api/admin/messages/analytics/route.ts")
    create_directory("app/api/admin/messages")
    create_file("app/api/admin/messages/route.ts")
    
    create_directory("app/api/admin/groups/[groupId]/members")
    create_file("app/api/admin/groups/[groupId]/members/route.ts")
    create_directory("app/api/admin/groups/[groupId]/messages")
    create_file("app/api/admin/groups/[groupId]/messages/route.ts")
    create_directory("app/api/admin/groups/[groupId]")
    create_file("app/api/admin/groups/[groupId]/route.ts")
    create_directory("app/api/admin/groups/analytics")
    create_file("app/api/admin/groups/analytics/route.ts")
    create_directory("app/api/admin/groups")
    create_file("app/api/admin/groups/route.ts")
    
    create_directory("app/api/admin/media/[fileId]")
    create_file("app/api/admin/media/[fileId]/route.ts")
    create_directory("app/api/admin/media/storage")
    create_file("app/api/admin/media/storage/route.ts")
    create_directory("app/api/admin/media/cleanup")
    create_file("app/api/admin/media/cleanup/route.ts")
    create_directory("app/api/admin/media")
    create_file("app/api/admin/media/route.ts")
    
    create_directory("app/api/admin/reports/[reportId]/resolve")
    create_file("app/api/admin/reports/[reportId]/resolve/route.ts")
    create_directory("app/api/admin/reports/[reportId]")
    create_file("app/api/admin/reports/[reportId]/route.ts")
    create_directory("app/api/admin/reports/types")
    create_file("app/api/admin/reports/types/route.ts")
    create_directory("app/api/admin/reports")
    create_file("app/api/admin/reports/route.ts")
    
    create_directory("app/api/admin/settings/system")
    create_file("app/api/admin/settings/system/route.ts")
    create_directory("app/api/admin/settings/notifications")
    create_file("app/api/admin/settings/notifications/route.ts")
    create_directory("app/api/admin/settings/security")
    create_file("app/api/admin/settings/security/route.ts")
    create_directory("app/api/admin/settings/maintenance")
    create_file("app/api/admin/settings/maintenance/route.ts")
    
    create_directory("app/api/admin/logs/errors")
    create_file("app/api/admin/logs/errors/route.ts")
    create_directory("app/api/admin/logs/audit")
    create_file("app/api/admin/logs/audit/route.ts")
    create_directory("app/api/admin/logs/export")
    create_file("app/api/admin/logs/export/route.ts")
    create_directory("app/api/admin/logs")
    create_file("app/api/admin/logs/route.ts")
    
    create_directory("app/api/admin/backup/create")
    create_file("app/api/admin/backup/create/route.ts")
    create_directory("app/api/admin/backup/restore")
    create_file("app/api/admin/backup/restore/route.ts")
    create_directory("app/api/admin/backup/list")
    create_file("app/api/admin/backup/list/route.ts")
    
    create_directory("app/api/webhook/smtp")
    create_file("app/api/webhook/smtp/route.ts")
    create_directory("app/api/webhook/push-notifications")
    create_file("app/api/webhook/push-notifications/route.ts")
    create_directory("app/api/webhook/payment")
    create_file("app/api/webhook/payment/route.ts")
    
    create_directory("app/api/health")
    create_file("app/api/health/route.ts")
    
    create_file("app/globals.css")
    
    # Lib directory and its subdirectories
    create_directory("lib/auth")
    create_file("lib/auth/jwt.ts")
    create_file("lib/auth/otp.ts")
    create_file("lib/auth/qr-auth.ts")
    create_file("lib/auth/middleware.ts")
    
    create_directory("lib/database/models")
    create_file("lib/database/models/user.ts")
    create_file("lib/database/models/chat.ts")
    create_file("lib/database/models/message.ts")
    create_file("lib/database/models/group.ts")
    create_file("lib/database/models/media.ts")
    create_file("lib/database/models/call.ts")
    create_file("lib/database/models/status.ts")
    create_file("lib/database/models/notification.ts")
    create_file("lib/database/models/report.ts")
    create_file("lib/database/models/admin.ts")
    
    create_directory("lib/database/schemas")
    create_file("lib/database/schemas/user.ts")
    create_file("lib/database/schemas/message.ts")
    create_file("lib/database/schemas/auth.ts")
    create_file("lib/database/schemas/media.ts")
    create_file("lib/database/schemas/group.ts")
    create_file("lib/database/schemas/call.ts")
    
    create_directory("lib/database/repositories")
    create_file("lib/database/repositories/user.ts")
    create_file("lib/database/repositories/chat.ts")
    create_file("lib/database/repositories/message.ts")
    create_file("lib/database/repositories/group.ts")
    create_file("lib/database/repositories/media.ts")
    create_file("lib/database/repositories/call.ts")
    create_file("lib/database/repositories/status.ts")
    
    create_file("lib/database/mongodb.ts")
    
    create_directory("lib/realtime/events")
    create_file("lib/realtime/events/messaging.ts")
    create_file("lib/realtime/events/presence.ts")
    create_file("lib/realtime/events/typing.ts")
    create_file("lib/realtime/events/calls.ts")
    create_file("lib/realtime/events/groups.ts")
    
    create_directory("lib/realtime/middleware")
    create_file("lib/realtime/middleware/auth.ts")
    create_file("lib/realtime/middleware/rate-limit.ts")
    
    create_file("lib/realtime/socket.ts")
    
    create_directory("lib/media")
    create_file("lib/media/s3.ts")
    create_file("lib/media/upload.ts")
    create_file("lib/media/compression.ts")
    create_file("lib/media/thumbnail.ts")
    create_file("lib/media/validation.ts")
    
    create_directory("lib/communication/templates")
    create_file("lib/communication/templates/email.ts")
    create_file("lib/communication/templates/sms.ts")
    
    create_file("lib/communication/smtp.ts")
    create_file("lib/communication/sms.ts")
    create_file("lib/communication/push-notifications.ts")
    
    create_directory("lib/webrtc")
    create_file("lib/webrtc/coturn.ts")
    create_file("lib/webrtc/signaling.ts")
    create_file("lib/webrtc/ice-candidates.ts")
    create_file("lib/webrtc/call-manager.ts")
    
    create_directory("lib/security")
    create_file("lib/security/encryption.ts")
    create_file("lib/security/rate-limiting.ts")
    create_file("lib/security/validation.ts")
    create_file("lib/security/sanitization.ts")
    create_file("lib/security/permissions.ts")
    
    create_directory("lib/utils")
    create_file("lib/utils/constants.ts")
    create_file("lib/utils/helpers.ts")
    create_file("lib/utils/date.ts")
    create_file("lib/utils/crypto.ts")
    create_file("lib/utils/pagination.ts")
    create_file("lib/utils/error-handler.ts")
    
    create_directory("lib/monitoring")
    create_file("lib/monitoring/analytics.ts")
    create_file("lib/monitoring/logging.ts")
    create_file("lib/monitoring/metrics.ts")
    create_file("lib/monitoring/health-check.ts")
    
    create_directory("lib/config")
    create_file("lib/config/environment.ts")
    create_file("lib/config/database.ts")
    create_file("lib/config/redis.ts")
    create_file("lib/config/cors.ts")
    create_file("lib/config/rate-limits.ts")
    
    # Root files
    create_file("middleware.ts")
    create_file("next.config.js")
    create_file("package.json")
    create_file("tsconfig.json")

if __name__ == "__main__":
    create_project_structure()
    print("Project structure created successfully!")