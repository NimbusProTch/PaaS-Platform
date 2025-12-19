import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { hashPassword, getSession } from '@/lib/auth'
import { z } from 'zod'

const registerSchema = z.object({
  email: z.string().email('Geçerli bir email adresi giriniz'),
  username: z.string().min(3, 'Kullanıcı adı en az 3 karakter olmalıdır'),
  password: z.string().min(6, 'Şifre en az 6 karakter olmalıdır'),
  firstName: z.string().optional(),
  lastName: z.string().optional(),
  phone: z.string().optional()
})

export async function POST(req: NextRequest) {
  try {
    const body = await req.json()

    // Validate request body
    const validatedData = registerSchema.parse(body)

    // Check if user exists
    const existingUser = await prisma.user.findFirst({
      where: {
        OR: [
          { email: validatedData.email },
          { username: validatedData.username }
        ]
      }
    })

    if (existingUser) {
      return NextResponse.json(
        { error: 'Bu email veya kullanıcı adı zaten kullanımda' },
        { status: 400 }
      )
    }

    // Hash password
    const hashedPassword = await hashPassword(validatedData.password)

    // Create user
    const user = await prisma.user.create({
      data: {
        email: validatedData.email,
        username: validatedData.username,
        password: hashedPassword,
        firstName: validatedData.firstName,
        lastName: validatedData.lastName,
        phone: validatedData.phone
      },
      select: {
        id: true,
        email: true,
        username: true,
        firstName: true,
        lastName: true,
        role: true
      }
    })

    // Create session
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
  } catch (error) {
    if (error instanceof z.ZodError) {
      return NextResponse.json(
        { error: error.errors[0].message },
        { status: 400 }
      )
    }

    console.error('Register error:', error)
    return NextResponse.json(
      { error: 'Kayıt işlemi sırasında bir hata oluştu' },
      { status: 500 }
    )
  }
}