import bcrypt from 'bcryptjs'
import jwt from 'jsonwebtoken'
import { getIronSession } from 'iron-session'
import { cookies } from 'next/headers'
import { prisma } from './prisma'

const JWT_SECRET = process.env.JWT_SECRET!
const SESSION_SECRET = process.env.SESSION_SECRET!

export interface SessionData {
  userId?: string
  email?: string
  username?: string
  role?: string
  isLoggedIn: boolean
}

export const sessionOptions = {
  password: SESSION_SECRET,
  cookieName: 'session',
  cookieOptions: {
    httpOnly: true,
    secure: process.env.NODE_ENV === 'production',
    sameSite: 'lax' as const,
    maxAge: 60 * 60 * 24 * 7 // 1 week
  }
}

export async function getSession() {
  const session = await getIronSession<SessionData>(await cookies(), sessionOptions)
  return session
}

export async function hashPassword(password: string): Promise<string> {
  return bcrypt.hash(password, 12)
}

export async function verifyPassword(password: string, hashedPassword: string): Promise<boolean> {
  return bcrypt.compare(password, hashedPassword)
}

export function generateToken(userId: string): string {
  return jwt.sign({ userId }, JWT_SECRET, { expiresIn: '7d' })
}

export function verifyToken(token: string): { userId: string } | null {
  try {
    return jwt.verify(token, JWT_SECRET) as { userId: string }
  } catch {
    return null
  }
}

export async function getUserFromSession() {
  const session = await getSession()
  if (!session.userId) return null

  return prisma.user.findUnique({
    where: { id: session.userId },
    select: {
      id: true,
      email: true,
      username: true,
      firstName: true,
      lastName: true,
      role: true,
      active: true
    }
  })
}