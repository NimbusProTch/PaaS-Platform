import { NextRequest, NextResponse } from 'next/server'

export async function POST(req: NextRequest) {
  try {
    const { email, password, username } = await req.json()

    // Try user-service first
    try {
      const response = await fetch(`${process.env.NEXT_PUBLIC_USER_SERVICE_URL || 'http://user-service.microservices:8081'}/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          username: username || email,
          password: password
        })
      })

      const data = await response.json()

      if (response.ok) {
        const res = NextResponse.json({
          success: true,
          user: data.user
        })

        if (data.token) {
          res.cookies.set('auth-token', data.token, {
            httpOnly: true,
            secure: process.env.NODE_ENV === 'production',
            sameSite: 'lax',
            maxAge: 60 * 60 * 24 * 7
          })
        }

        return res
      }
    } catch (serviceError) {
      console.log('User service not available, using mock login')
    }

    // Fallback mock authentication for demo
    if ((email === 'demo@test.com' || username === 'demo') && password === 'demo123') {
      const user = {
        id: '1',
        email: 'demo@test.com',
        username: 'demo',
        firstName: 'Demo',
        lastName: 'User',
        role: 'USER'
      }

      return NextResponse.json({
        success: true,
        user
      })
    }

    return NextResponse.json(
      { error: 'Kullanıcı adı veya şifre hatalı' },
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