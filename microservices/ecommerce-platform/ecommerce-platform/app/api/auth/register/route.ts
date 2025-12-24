import { NextRequest, NextResponse } from 'next/server'

export async function POST(req: NextRequest) {
  try {
    const body = await req.json()

    // Call user-service register endpoint
    const response = await fetch(`${process.env.NEXT_PUBLIC_USER_SERVICE_URL || 'http://user-service.microservices:8081'}/register`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        email: body.email,
        username: body.username,
        password: body.password,
        firstName: body.firstName,
        lastName: body.lastName,
        phone: body.phone
      })
    })

    const data = await response.json()

    if (!response.ok) {
      return NextResponse.json(
        { error: data.error || 'Kayıt işlemi başarısız' },
        { status: response.status }
      )
    }

    // Set session cookies if needed
    const res = NextResponse.json({
      success: true,
      user: data.user
    })

    // You can set cookies here if user-service returns a token
    if (data.token) {
      res.cookies.set('auth-token', data.token, {
        httpOnly: true,
        secure: process.env.NODE_ENV === 'production',
        sameSite: 'lax',
        maxAge: 60 * 60 * 24 * 7 // 7 days
      })
    }

    return res
  } catch (error) {
    console.error('Register error:', error)
    return NextResponse.json(
      { error: 'Kayıt işlemi sırasında bir hata oluştu' },
      { status: 500 }
    )
  }
}