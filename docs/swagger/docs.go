package swagger

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/healthz": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "system"
                ],
                "summary": "Health check",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/internal/review-queue": {
            "post": {
                "description": "Laravel internal endpoint used by the Go review outbox. ` + "`" + `X-AI-Signature` + "`" + ` is lowercase hex HMAC-SHA256 over ` + "`" + `\u003ctimestamp\u003e.\u003craw JSON body\u003e` + "`" + ` using ` + "`" + `GO_AI_WEBHOOK_SECRET` + "`" + `.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "laravel-callbacks"
                ],
                "summary": "Receive signed Laravel review queue callback",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Unix seconds timestamp, accepted within GO_AI_WEBHOOK_TOLERANCE",
                        "name": "X-AI-Timestamp",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Lowercase hex HMAC-SHA256 of '\u003ctimestamp\u003e.\u003craw body\u003e'",
                        "name": "X-AI-Signature",
                        "in": "header",
                        "required": true
                    },
                    {
                        "description": "Review queue callback",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.reviewQueueCallbackRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Existing idempotent review item",
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.reviewQueueCallbackResponse"
                        }
                    },
                    "201": {
                        "description": "Created review queue item",
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.reviewQueueCallbackResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "422": {
                        "description": "Unprocessable Entity",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "503": {
                        "description": "Service Unavailable",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        },
        "/v1/admin/review-queue/{id}/reject": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "review-queue"
                ],
                "summary": "Reject review queue item",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Bearer \u003cRS256 service JWT\u003e",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Tenant ID",
                        "name": "X-Tenant-Id",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "Review item ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Review rejection",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.reviewRejectRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        },
        "/v1/inbound/email": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "inbound"
                ],
                "summary": "Receive email inbound webhook",
                "parameters": [
                    {
                        "description": "Email webhook payload",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.emailInboundWebhookRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        },
        "/v1/inbound/line": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "inbound"
                ],
                "summary": "Receive LINE inbound webhook",
                "parameters": [
                    {
                        "description": "LINE webhook payload",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.lineInboundWebhookRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        },
        "/v1/reply": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "reply"
                ],
                "summary": "Create reply",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Bearer \u003cRS256 service JWT\u003e",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Tenant ID",
                        "name": "X-Tenant-Id",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Request budget in milliseconds; Go runs within this minus headroom, falling back to server config when absent",
                        "name": "X-Timeout-Ms",
                        "in": "header"
                    },
                    {
                        "description": "Reply request",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_application_dto.ReplyRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "allOf": [
                                {
                                    "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                                },
                                {
                                    "type": "object",
                                    "properties": {
                                        "data": {
                                            "$ref": "#/definitions/github_com_my_app_internal_application_dto.ReplyResponse"
                                        }
                                    }
                                }
                            ]
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        },
        "/v1/reply/feedback": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "reply"
                ],
                "summary": "Record reply feedback",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Bearer \u003cRS256 service JWT\u003e",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Tenant ID",
                        "name": "X-Tenant-Id",
                        "in": "header",
                        "required": true
                    },
                    {
                        "description": "Smart-reply feedback",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/scripts_swagger.feedbackRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Envelope"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "github_com_my_app_internal_application_dto.AttachmentRef": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                },
                "mime_type": {
                    "type": "string"
                },
                "size_bytes": {
                    "type": "integer",
                    "minimum": 0
                },
                "storage_key": {
                    "type": "string"
                },
                "url": {
                    "type": "string"
                }
            }
        },
        "github_com_my_app_internal_application_dto.ReplyRequest": {
            "type": "object",
            "properties": {
                "attachments": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/github_com_my_app_internal_application_dto.AttachmentRef"
                    }
                },
                "channel": {
                    "type": "string",
                    "enum": [
                        "line",
                        "email",
                        "web",
                        "voice",
                        "api",
                        "in_app"
                    ],
                    "example": "web"
                },
                "conversation_id": {
                    "type": "string",
                    "example": "conv_8f21"
                },
                "customer_id": {
                    "type": "string",
                    "example": "cust_10293"
                },
                "debug": {
                    "type": "boolean",
                    "example": false
                },
                "message": {
                    "type": "string",
                    "example": "My internet has been offline since this morning."
                },
                "message_id": {
                    "type": "string",
                    "example": "msg_1001"
                },
                "metadata": {
                    "type": "object",
                    "additionalProperties": {
                        "type": "string"
                    }
                },
                "mode": {
                    "type": "string",
                    "enum": [
                        "fast_draft",
                        "full_review",
                        "debug"
                    ],
                    "example": "full_review"
                },
                "query": {
                    "type": "string"
                },
                "site_id": {
                    "type": "string",
                    "example": "site_42"
                }
            }
        },
        "github_com_my_app_internal_application_dto.ReplyResponse": {
            "type": "object",
            "properties": {
                "ai_action_id": {
                    "type": "integer"
                },
                "confidence": {
                    "type": "number"
                },
                "debug_trace": {},
                "decision": {
                    "type": "string"
                },
                "draft": {
                    "type": "string"
                },
                "intent": {
                    "type": "string"
                },
                "reason": {
                    "type": "string"
                },
                "retry_after_ms": {
                    "type": "integer"
                },
                "sources": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/github_com_my_app_internal_application_dto.ReplySource"
                    }
                },
                "stage_timings_ms": {
                    "type": "object",
                    "additionalProperties": {
                        "type": "integer",
                        "format": "int64"
                    }
                },
                "suggestion": {
                    "type": "string"
                }
            }
        },
        "github_com_my_app_internal_application_dto.ReplySource": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                },
                "score": {
                    "type": "number"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "github_com_my_app_internal_transport_http_response.Envelope": {
            "type": "object",
            "properties": {
                "data": {},
                "error": {
                    "$ref": "#/definitions/github_com_my_app_internal_transport_http_response.Error"
                },
                "success": {
                    "type": "boolean"
                }
            }
        },
        "github_com_my_app_internal_transport_http_response.Error": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string"
                },
                "message": {
                    "type": "string"
                }
            }
        },
        "scripts_swagger.emailInboundWebhookRequest": {
            "type": "object",
            "properties": {
                "body": {
                    "type": "string",
                    "example": "The connection has been unstable since this morning."
                },
                "body_html": {
                    "type": "string",
                    "example": "\u003cp\u003eThe connection has been unstable since this morning.\u003c/p\u003e"
                },
                "from": {
                    "type": "string",
                    "example": "Customer One \u003ccustomer@example.com\u003e"
                },
                "message_id": {
                    "type": "string",
                    "example": "email-20260520-001"
                },
                "subject": {
                    "type": "string",
                    "example": "Internet connection issue"
                },
                "to": {
                    "type": "string",
                    "example": "support@example.com"
                }
            }
        },
        "scripts_swagger.feedbackRequest": {
            "type": "object",
            "properties": {
                "ai_action_id": {
                    "type": "integer",
                    "example": 42
                },
                "note": {
                    "type": "string",
                    "example": "Agent sent the draft unchanged."
                },
                "verdict": {
                    "type": "string",
                    "enum": [
                        "accepted",
                        "edited",
                        "rejected",
                        "escalated"
                    ],
                    "example": "accepted"
                }
            }
        },
        "scripts_swagger.lineInboundEvent": {
            "type": "object",
            "properties": {
                "message": {
                    "$ref": "#/definitions/scripts_swagger.lineInboundMessage"
                },
                "replyToken": {
                    "type": "string",
                    "example": "reply-token-example"
                },
                "source": {
                    "$ref": "#/definitions/scripts_swagger.lineInboundSource"
                },
                "timestamp": {
                    "type": "integer",
                    "example": 1716172800000
                },
                "type": {
                    "type": "string",
                    "example": "message"
                }
            }
        },
        "scripts_swagger.lineInboundMessage": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string",
                    "example": "519551372899"
                },
                "text": {
                    "type": "string",
                    "example": "My internet is offline"
                },
                "type": {
                    "type": "string",
                    "example": "text"
                }
            }
        },
        "scripts_swagger.lineInboundSource": {
            "type": "object",
            "properties": {
                "groupId": {
                    "type": "string",
                    "example": "C4af4980629"
                },
                "roomId": {
                    "type": "string",
                    "example": "R4af4980629"
                },
                "type": {
                    "type": "string",
                    "example": "user"
                },
                "userId": {
                    "type": "string",
                    "example": "U4af4980629"
                }
            }
        },
        "scripts_swagger.lineInboundWebhookRequest": {
            "type": "object",
            "properties": {
                "destination": {
                    "type": "string",
                    "example": "U1234567890abcdef1234567890abcdef"
                },
                "events": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/scripts_swagger.lineInboundEvent"
                    }
                }
            }
        },
        "scripts_swagger.reviewQueueCallbackData": {
            "type": "object",
            "properties": {
                "company_id": {
                    "type": "integer",
                    "example": 1
                },
                "id": {
                    "type": "integer",
                    "example": 123
                },
                "kind": {
                    "type": "string",
                    "example": "symptom_proposed"
                }
            }
        },
        "scripts_swagger.reviewQueueCallbackRequest": {
            "type": "object",
            "properties": {
                "company_id": {
                    "type": "integer",
                    "example": 1
                },
                "kind": {
                    "type": "string",
                    "enum": [
                        "kb_promotion",
                        "symptom_proposed",
                        "subject_proposed",
                        "kb_gap"
                    ],
                    "example": "symptom_proposed"
                },
                "payload": {
                    "type": "object",
                    "additionalProperties": true
                },
                "payload_hash": {
                    "type": "string",
                    "example": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
                },
                "source_refs": {
                    "type": "object",
                    "additionalProperties": true
                }
            }
        },
        "scripts_swagger.reviewQueueCallbackResponse": {
            "type": "object",
            "properties": {
                "data": {
                    "$ref": "#/definitions/scripts_swagger.reviewQueueCallbackData"
                }
            }
        },
        "scripts_swagger.reviewRejectRequest": {
            "type": "object",
            "properties": {
                "reason": {
                    "type": "string",
                    "example": "The generated article duplicated an existing answer."
                }
            }
        }
    }
}`

var SwaggerInfo = &swag.Spec{
	Version:          "0.1.0",
	Host:             "",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "Centric RAG AI Service",
	Description:      "Internal AI service for tenancy-scoped support retrieval and reply workflows.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
