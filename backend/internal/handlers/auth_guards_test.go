package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/config"
)

// Регрессия SEC-001 / SEC-007.
//
// Эти эндпоинты раньше обрабатывали анонимные запросы: /api/users.php отдавал
// колонку password всех сотрудников и позволял менять чужой пароль и user_group,
// /api/profile.php редактировал чужую карточку по id из тела, /api/forms*.php
// отдавал и удалял формы и отчёты, /api/ofo*.php — оргструктуру, а права
// администратора форм определялись по клиентскому заголовку X-User-Id.
//
// Pool во всех хендлерах намеренно nil: корректная проверка сессии обязана
// завершиться 401 ДО любого обращения к БД. Если проверка пропущена, тест
// падает паникой на nil-пуле — это и есть сигнал регрессии.
func TestEndpointsRequireSessionBeforeTouchingDB(t *testing.T) {
	authSvc := &auth.Service{Pool: nil}
	usersH := &Users{Pool: nil, Auth: authSvc}
	profileH := &Profile{Pool: nil, Auth: authSvc}
	wallH := &ProfileWall{Pool: nil, Auth: authSvc}
	extrasH := &ProfileExtras{Pool: nil, Auth: authSvc}
	avatarH := &ProfileAvatar{Pool: nil, Auth: authSvc}
	uploadH := &Upload{Auth: authSvc}
	coursesH := &CoursesHandler{Auth: authSvc}
	rsvpH := &EventRSVP{Pool: nil, Auth: authSvc}
	calendarH := &CalendarEntries{Pool: nil, Auth: authSvc}
	formsH := &FormsHandler{Pool: nil, Auth: authSvc}
	ofoH := &OFO{Pool: nil, Auth: authSvc}
	notificationsH := &Notifications{Pool: nil, Auth: authSvc}
	commentsH := &NewsComments{Pool: nil, Auth: authSvc}
	devblogH := &Devblog{Pool: nil, Auth: authSvc}

	cases := []struct {
		name    string
		method  string
		target  string
		body    string
		headers map[string]string
		serve   func(http.ResponseWriter, *http.Request)
	}{
		{"users list", http.MethodGet, "/api/users.php", "", nil, usersH.ServeHTTP},
		{"users update", http.MethodPut, "/api/users.php?id=1", `{"user_group":"admin"}`, nil, usersH.ServeHTTP},
		{"profile read", http.MethodGet, "/api/profile.php?id=1", "", nil, profileH.ServeHTTP},
		{"profile write", http.MethodPost, "/api/profile.php", `{"id":1,"role":"x"}`, nil, profileH.ServeHTTP},
		{"wall read", http.MethodGet, "/api/profile_wall.php?userId=1", "", nil, wallH.ServeHTTP},
		{"wall create", http.MethodPost, "/api/profile_wall.php", `{"action":"create","userId":1,"content":"x"}`, nil, wallH.ServeHTTP},
		{"wall delete", http.MethodPost, "/api/profile_wall.php", `{"action":"delete","id":1}`, nil, wallH.ServeHTTP},
		{"wall greet", http.MethodPost, "/api/profile_wall.php", `{"action":"greet","userId":1,"content":"x"}`, nil, wallH.ServeHTTP},
		{"wall my greetings", http.MethodGet, "/api/profile_wall.php?action=my_greetings", "", nil, wallH.ServeHTTP},
		{"notifications list", http.MethodGet, "/api/notifications.php", "", nil, notificationsH.ServeHTTP},
		{"notifications read", http.MethodPost, "/api/notifications.php", `{"action":"read","ids":[1]}`, nil, notificationsH.ServeHTTP},
		{"notifications read all", http.MethodPost, "/api/notifications.php", `{"action":"read_all"}`, nil, notificationsH.ServeHTTP},
		{"comments list", http.MethodGet, "/api/news_comments.php?newsId=1", "", nil, commentsH.ServeHTTP},
		{"comments replies", http.MethodGet, "/api/news_comments.php?action=replies&rootId=1", "", nil, commentsH.ServeHTTP},
		{"comments thread", http.MethodGet, "/api/news_comments.php?action=thread&id=1", "", nil, commentsH.ServeHTTP},
		{"comment create", http.MethodPost, "/api/news_comments.php", `{"action":"create","newsId":1,"content":"x"}`, nil, commentsH.ServeHTTP},
		{"comment reply", http.MethodPost, "/api/news_comments.php", `{"action":"create","replyToId":1,"content":"x"}`, nil, commentsH.ServeHTTP},
		{"comment delete", http.MethodPost, "/api/news_comments.php", `{"action":"delete","id":1}`, nil, commentsH.ServeHTTP},
		{"comment react", http.MethodPost, "/api/news_comments.php", `{"action":"react","id":1,"reaction":"like","active":true}`, nil, commentsH.ServeHTTP},
		{"devblog pending", http.MethodGet, "/api/devblog.php", "", nil, devblogH.ServeHTTP},
		{"devblog draft", http.MethodGet, "/api/devblog.php?action=draft", "", nil, devblogH.ServeHTTP},
		{"devblog save", http.MethodPost, "/api/devblog.php", `{"action":"save","title":"x","body":"x","version":0}`, nil, devblogH.ServeHTTP},
		{"devblog publish", http.MethodPost, "/api/devblog.php", `{"action":"publish","title":"x","body":"x","html":"<p>x</p>","version":0}`, nil, devblogH.ServeHTTP},
		{"devblog dismiss", http.MethodPost, "/api/devblog.php", `{"action":"dismiss","newsId":1}`, nil, devblogH.ServeHTTP},
		{"avatar upload", http.MethodPost, "/api/profile_avatar.php", `{}`, nil, avatarH.ServeHTTP},
		{"image upload", http.MethodPost, "/api/Upload/upload.php", `{}`, nil, uploadH.ServeHTTP},
		{"rsvp list", http.MethodGet, "/api/event_rsvp.php", "", nil, rsvpH.ServeHTTP},
		{"rsvp set", http.MethodPost, "/api/event_rsvp.php", `{"eventId":1,"joined":true}`, nil, rsvpH.ServeHTTP},
		{"rsvp participants", http.MethodGet, "/api/event_rsvp.php?eventId=1", "", nil, rsvpH.ServeHTTP},
		{"calendar list", http.MethodGet, "/api/calendar_entries.php", "", nil, calendarH.ServeHTTP},
		{"calendar create", http.MethodPost, "/api/calendar_entries.php", `{"action":"create","source":"personal","dateKey":"2026-10-02","title":"x"}`, nil, calendarH.ServeHTTP},
		{"calendar delete", http.MethodPost, "/api/calendar_entries.php", `{"action":"delete","id":1}`, nil, calendarH.ServeHTTP},
		{"calendar update", http.MethodPost, "/api/calendar_entries.php", `{"action":"update","id":1,"source":"personal","dateKey":"2026-10-02","title":"x","color":"#3b82f6"}`, nil, calendarH.ServeHTTP},
		{"course assignment cancel", http.MethodPost, "/api/course_assignment_cancel.php", `{}`, nil, coursesH.AssignmentCancel},
		{"course assignments list", http.MethodPost, "/api/course_assignments_list.php", `{}`, nil, coursesH.AssignmentsList},
		{"course history", http.MethodPost, "/api/course_history.php", `{}`, nil, coursesH.History},
		{"course materials order", http.MethodPost, "/api/course_materials_order.php", `{}`, nil, coursesH.MaterialsOrder},
		{"courses archive", http.MethodPost, "/api/courses_archive.php", `{}`, nil, coursesH.Archive},
		{"courses duplicate", http.MethodPost, "/api/courses_duplicate.php", `{}`, nil, coursesH.Duplicate},
		{"courses readiness", http.MethodPost, "/api/courses_readiness.php", `{}`, nil, coursesH.Readiness},
		{"wish add", http.MethodPost, "/api/profile_extras.php", `{"action":"wish_add","text":"x"}`, nil, extrasH.ServeHTTP},
		{"award add", http.MethodPost, "/api/profile_extras.php", `{"action":"award_add","userId":1,"title":"x","awardedOn":"2026-10-01"}`, nil, extrasH.ServeHTTP},
		{"forms create", http.MethodPost, "/api/forms.php", `{}`, nil, formsH.Forms},
		{"forms read", http.MethodGet, "/api/forms.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", "", nil, formsH.Forms},
		{"forms update", http.MethodPut, "/api/forms.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", `{}`, nil, formsH.Forms},
		{"forms list", http.MethodGet, "/api/forms_list.php", "", nil, formsH.List},
		{"forms publish", http.MethodPost, "/api/forms_publish.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", "", nil, formsH.Publish},
		{"forms report", http.MethodGet, "/api/forms_report.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", "", nil, formsH.Report},
		{"ofo list", http.MethodGet, "/api/ofo.php", "", nil, ofoH.List},
		{"ofo seats", http.MethodGet, "/api/ofo_seats.php", "", nil, ofoH.Seats},
		{"ofo tree", http.MethodGet, "/api/ofo_tree.php", "", nil, ofoH.Tree},
		{"ofo positions", http.MethodGet, "/api/ofo_positions.php", "", nil, ofoH.Positions},
		{"ofo positions add", http.MethodPost, "/api/ofo_positions.php", `{"unit_number":1,"name":"x"}`, nil, ofoH.Positions},

		// SEC-007: заголовок X-User-Id больше не является удостоверением личности.
		{"forms list with spoofed X-User-Id", http.MethodGet, "/api/forms_list.php", "",
			map[string]string{"X-User-Id": "1"}, formsH.List},
		{"forms report with spoofed X-User-Id", http.MethodGet, "/api/forms_report.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", "",
			map[string]string{"X-User-Id": "1"}, formsH.Report},
		{"forms archive with spoofed X-User-Id", http.MethodPost, "/api/forms_archive.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", `{"status":"archived"}`,
			map[string]string{"X-User-Id": "1"}, formsH.Archive},
		{"forms delete with spoofed X-User-Id", http.MethodDelete, "/api/forms_delete.php?id=0f9f9c4a-5a2c-4d4e-8b9a-2b1c3d4e5f60", "",
			map[string]string{"X-User-Id": "1"}, formsH.Delete},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.target, nil)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			tc.serve(rec, req)

			if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
				t.Fatalf("ожидался 401/403 для анонимного запроса, получен %d (тело: %s)",
					rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"password"`) {
				t.Fatalf("в ответе анонимному клиенту присутствует поле password")
			}
		})
	}
}

// Регрессия SEC-008: модуль тестов (test_forms) брал личность из userId в теле/query.
//
// Подставив userId владельца, злоумышленник мог редактировать, публиковать, снимать
// с публикации и удалять чужие формы, читать ответы участников и статистику. Теперь
// личность берётся только из сессии; userId в теле игнорируется. Без сессии
// операции над формами обязаны отвечать 401 ДО обращения к БД (Pool = nil).
func TestTestsModuleIgnoresClientUserID(t *testing.T) {
	h := &TestsHandler{Pool: nil, Auth: &auth.Service{Pool: nil}}

	cases := []struct {
		name  string
		body  string
		serve func(http.ResponseWriter, *http.Request)
	}{
		{"save spoof", `{"userId":1,"form":{"title":"x"}}`, h.Save},
		{"publish spoof", `{"userId":1,"form":{"title":"x"}}`, h.Publish},
		{"unpublish spoof", `{"userId":1,"formId":1}`, h.Unpublish},
		{"delete spoof", `{"userId":1,"formId":1}`, h.Delete},
		{"direct spoof", `{"userId":1,"formId":1,"mode":"users","ids":[2]}`, h.Direct},
		{"stats spoof", `{"userId":1,"formId":1}`, h.Stats},
		{"participant spoof", `{"userId":1,"formId":1,"participantId":2}`, h.Participant},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/tests_x.php", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			// И заголовок X-User-Id тоже не должен признаваться удостоверением.
			req.Header.Set("X-User-Id", "1")
			rec := httptest.NewRecorder()

			tc.serve(rec, req)

			if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
				t.Fatalf("%s: ожидался 401/403 при подделке userId, получен %d (тело: %s)",
					tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// Регрессия SEC-009: /api/absence_journal.php был полностью открыт — аноним мог
// читать ФИО и причины отсутствия всех сотрудников (GET), а также создавать,
// изменять и удалять любые записи (POST/PUT/DELETE). Все методы теперь требуют
// авторизации ДО обращения к БД (Pool = nil).
func TestAbsenceJournalRequiresAuth(t *testing.T) {
	h := &Absence{Pool: nil, Auth: &auth.Service{Pool: nil}}

	// ServeHTTP выполняет SET TIME ZONE на nil-пуле, поэтому дёргаем методы напрямую.
	cases := []struct {
		name   string
		method string
		target string
		body   string
		serve  func(http.ResponseWriter, *http.Request)
	}{
		{"list", http.MethodGet, "/api/absence_journal.php?limit=500", "", h.get},
		{"create", http.MethodPost, "/api/absence_journal.php", `{"user_id":1,"fio":"x","start_datetime":"2026-01-01T00:00"}`, h.post},
		{"update", http.MethodPut, "/api/absence_journal.php?id=1", `{}`, h.put},
		{"delete", http.MethodDelete, "/api/absence_journal.php?id=1", "", h.del},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.target, nil)
			}
			rec := httptest.NewRecorder()
			tc.serve(rec, req)
			if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
				t.Fatalf("%s: ожидался 401/403, получен %d (тело: %s)", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// Регрессия SEC-001: /api/sync.php создавал пользователя с произвольным логином
// и паролем без какого-либо секрета — этого достаточно, чтобы затем войти в портал.
func TestSyncRequiresSharedSecret(t *testing.T) {
	h := &Sync{Pool: nil, Config: config.Config{ASUSecret: "test-secret"}}
	body := `{"action":"create","user":{"id":424242,"name":"Тест Тест","login":"intruder","password":"p"}}`

	cases := []struct {
		name   string
		secret string
	}{
		{"без секрета", ""},
		{"неверный секрет", "wrong"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/sync.php", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.secret != "" {
				req.Header.Set("X-Sync-Secret", tc.secret)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("ожидался 401, получен %d (тело: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// Пустой ASU_SECRET не должен означать «пускать всех».
func TestSyncRejectsWhenSecretNotConfigured(t *testing.T) {
	h := &Sync{Pool: nil, Config: config.Config{ASUSecret: ""}}
	req := httptest.NewRequest(http.MethodPost, "/api/sync.php",
		strings.NewReader(`{"action":"create","user":{"id":1,"login":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ожидался 401 при незаданном секрете, получен %d", rec.Code)
	}
}
