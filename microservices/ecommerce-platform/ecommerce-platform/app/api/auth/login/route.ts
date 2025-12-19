import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { verifyPassword, getSession } from '@/lib/auth'

export async function POST(req: NextRequest) {
  try {
    const { email, password } = await req.json()

    // Basit mock authentication - gerçek veritabanı bağlantısı olmadan
    if (email === 'test@test.com' && password === '123456') {
      // Mock user data
      const user = {
        id: '1',
        email: 'test@test.com',
        username: 'testuser',
        firstName: 'Test',
        lastName: 'User',
        role: 'USER'
      }

      // Session oluştur
      const session = await getSession()
      session.userId = user.id
      session.email = user.email
      session.username = user.username
      session.role = user.role
      session.isLoggedIn = true
      await session.save()

      return NextResponse.json({
        success: true,
        user
      })
    }

    return NextResponse.json(
      { error: 'E-posta veya şifre hatalı' },
      { status: 401 }
    )
  } catch (error) {
    console.error('Login error:', error)
    return NextResponse.json(
      { error: 'Giriş işlemi sırasında bir hata oluştu' },
      { status: 500 }
    )
  }
}