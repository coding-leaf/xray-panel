import { toast } from './toast'

export async function copyText(text: string, successMessage = '已复制到剪贴板'): Promise<boolean> {
  if (!text) return false
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.focus()
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    toast.success(successMessage)
    return true
  } catch (err) {
    toast.error('复制失败，请手动选中文本复制')
    return false
  }
}

export const copyToClipboard = copyText
