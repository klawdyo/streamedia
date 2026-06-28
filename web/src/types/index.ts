// Tipos compartilhados do admin Streamedia



export interface User {

  id: number

  email: string

  name: string

  picture: string

  created_at: string

}



export interface UserRole {

  role: string

  level_num: number

}



export interface UserWithRoles extends User {

  roles: UserRole[]

  effective_level: number

}



export interface Video {

  video_id: string

  status: string

  tag: string

  actual_size_bytes?: number

  duration_s?: number

  created_at: string

  updated_at: string

}



export interface VideoStatusResponse {

  video_id: string

  status: string

  tag: string

  actual_size_bytes?: number

  duration_s?: number

  created_at: string

  updated_at: string

}



export interface ConfigGroup {

  key: string

  title: string

  description?: string

  items: ConfigItem[]

}



export interface ConfigItem {

  key: string

  value?: string

  type: string

  description: string

  validation?: string

  default?: string

  visible: boolean

}



export interface AuthResponse {

  email: string

  name: string

  picture: string

  roles: UserRole[]

  effective_level: number

}



export interface ApiResponse<T> {

  data: T

  error?: string

  meta?: {

  page: number

  per_page: number

  total: number

  total_pages: number

  }

}



export interface PaginatedResponse<T> {

  data: T[]

  meta: {

  page: number

  per_page: number

  total: number

  total_pages: number

  }

}



export interface StatsResponse {

  total_videos: number

  total_size_bytes: number

  ready_videos: number

  processing_videos: number

  failed_videos: number

}



export interface QueueResponse {

  queue_length: number

  processing: number

  pending: number

}



export interface SSEEvent {

  event: string

  video_id: string

  tag?: string

  status?: string

  timestamp: string

  data?: Record<string, unknown>

}



export interface UploadInitResponse {

  upload_id: string

  location: string

  video_id?: string

}



export interface PlayInitResponse {

  hls_url: string

  video_id: string

  status: string

}