'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { ShoppingCart, Package, User, LogOut, Search, Plus, Minus } from 'lucide-react'

interface Product {
  id: string
  name: string
  description: string
  price: number
  stock: number
  imageUrl: string
  category: string
}

interface CartItem {
  product: Product
  quantity: number
}

export default function Dashboard() {
  const router = useRouter()
  const [products] = useState<Product[]>([
    {
      id: '1',
      name: 'MacBook Pro M3',
      description: '14-inch, M3 Pro chip, 18GB RAM, 512GB SSD',
      price: 89999,
      stock: 5,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/mbp14-m3-max-pro-spaceblack-select-202310?wid=904&hei=840&fmt=jpeg&qlt=90&.v=1697230830200',
      category: 'Bilgisayar'
    },
    {
      id: '2',
      name: 'iPhone 15 Pro',
      description: '256GB, Titanium Blue',
      price: 64999,
      stock: 12,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/iphone-15-pro-finish-select-202309-6-1inch-bluetitanium?wid=2560&hei=1440&fmt=p-jpg&qlt=80&.v=1692846360609',
      category: 'Telefon'
    },
    {
      id: '3',
      name: 'AirPods Pro',
      description: '2. Nesil, USB-C',
      price: 10499,
      stock: 25,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/MTJV3?wid=1144&hei=1144&fmt=jpeg&qlt=90&.v=1694014871985',
      category: 'Aksesuar'
    },
    {
      id: '4',
      name: 'iPad Pro',
      description: '12.9-inch, M2 chip, 256GB',
      price: 45999,
      stock: 8,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/ipad-pro-13-select-wifi-spacegray-202210?wid=940&hei=1112&fmt=p-jpg&qlt=95&.v=1664411207213',
      category: 'Tablet'
    },
    {
      id: '5',
      name: 'Apple Watch Ultra 2',
      description: 'Titanium Case, Alpine Loop',
      price: 34999,
      stock: 7,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/s9-case-titanium-natural-band-ocean-202409?wid=940&hei=1112&fmt=p-jpg&qlt=95&.v=1724936934290',
      category: 'Aksesuar'
    },
    {
      id: '6',
      name: 'Magic Keyboard',
      description: 'iPad Pro 12.9-inch için',
      price: 8999,
      stock: 15,
      imageUrl: 'https://store.storeimages.cdn-apple.com/4982/as-images.apple.com/is/MJQL3T?wid=1144&hei=1144&fmt=jpeg&qlt=90&.v=1621538312000',
      category: 'Aksesuar'
    }
  ])

  const [cart, setCart] = useState<CartItem[]>([])
  const [showCart, setShowCart] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [selectedCategory, setSelectedCategory] = useState('Tümü')

  const categories = ['Tümü', 'Bilgisayar', 'Telefon', 'Tablet', 'Aksesuar']

  const filteredProducts = products.filter(product => {
    const matchesSearch = product.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
                          product.description.toLowerCase().includes(searchTerm.toLowerCase())
    const matchesCategory = selectedCategory === 'Tümü' || product.category === selectedCategory
    return matchesSearch && matchesCategory
  })

  const addToCart = (product: Product) => {
    setCart(prevCart => {
      const existingItem = prevCart.find(item => item.product.id === product.id)
      if (existingItem) {
        return prevCart.map(item =>
          item.product.id === product.id
            ? { ...item, quantity: item.quantity + 1 }
            : item
        )
      }
      return [...prevCart, { product, quantity: 1 }]
    })
  }

  const updateCartQuantity = (productId: string, delta: number) => {
    setCart(prevCart => {
      return prevCart.map(item => {
        if (item.product.id === productId) {
          const newQuantity = item.quantity + delta
          return newQuantity > 0 ? { ...item, quantity: newQuantity } : null
        }
        return item
      }).filter(Boolean) as CartItem[]
    })
  }

  const getTotalPrice = () => {
    return cart.reduce((total, item) => total + (item.product.price * item.quantity), 0)
  }

  const handleLogout = async () => {
    await fetch('/api/auth/logout', { method: 'POST' })
    router.push('/')
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center">
              <Package className="h-8 w-8 text-blue-600 mr-3" />
              <h1 className="text-xl font-bold text-gray-900">E-Ticaret Platformu</h1>
            </div>

            <div className="flex items-center space-x-4">
              <button
                onClick={() => setShowCart(!showCart)}
                className="relative p-2 text-gray-600 hover:text-blue-600 transition"
              >
                <ShoppingCart className="h-6 w-6" />
                {cart.length > 0 && (
                  <span className="absolute -top-1 -right-1 bg-red-500 text-white text-xs rounded-full h-5 w-5 flex items-center justify-center">
                    {cart.reduce((sum, item) => sum + item.quantity, 0)}
                  </span>
                )}
              </button>

              <button className="p-2 text-gray-600 hover:text-blue-600 transition">
                <User className="h-6 w-6" />
              </button>

              <button
                onClick={handleLogout}
                className="p-2 text-gray-600 hover:text-red-600 transition"
              >
                <LogOut className="h-6 w-6" />
              </button>
            </div>
          </div>
        </div>
      </header>

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Search and Filters */}
        <div className="mb-8 space-y-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-5 w-5" />
            <input
              type="text"
              placeholder="Ürün ara..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full pl-10 pr-4 py-3 border border-gray-200 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
            />
          </div>

          <div className="flex space-x-2 overflow-x-auto pb-2">
            {categories.map(category => (
              <button
                key={category}
                onClick={() => setSelectedCategory(category)}
                className={`px-4 py-2 rounded-lg font-medium whitespace-nowrap transition ${
                  selectedCategory === category
                    ? 'bg-blue-600 text-white'
                    : 'bg-white text-gray-600 hover:bg-gray-100'
                }`}
              >
                {category}
              </button>
            ))}
          </div>
        </div>

        {/* Products Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredProducts.map(product => (
            <div key={product.id} className="bg-white rounded-xl shadow-md overflow-hidden hover:shadow-xl transition">
              <img
                src={product.imageUrl}
                alt={product.name}
                className="w-full h-48 object-cover"
              />
              <div className="p-6">
                <h3 className="text-lg font-semibold text-gray-900">{product.name}</h3>
                <p className="text-sm text-gray-500 mt-1">{product.description}</p>
                <div className="mt-4 flex items-center justify-between">
                  <div>
                    <p className="text-2xl font-bold text-gray-900">
                      ₺{product.price.toLocaleString('tr-TR')}
                    </p>
                    <p className="text-sm text-gray-500">Stok: {product.stock}</p>
                  </div>
                  <button
                    onClick={() => addToCart(product)}
                    disabled={product.stock === 0}
                    className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed transition"
                  >
                    Sepete Ekle
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Cart Sidebar */}
      <div className={`fixed right-0 top-0 h-full w-96 bg-white shadow-xl transform transition-transform z-50 ${
        showCart ? 'translate-x-0' : 'translate-x-full'
      }`}>
        <div className="p-6 border-b">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold">Sepetim</h2>
            <button
              onClick={() => setShowCart(false)}
              className="text-gray-400 hover:text-gray-600"
            >
              ✕
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {cart.length === 0 ? (
            <p className="text-gray-500 text-center mt-8">Sepetiniz boş</p>
          ) : (
            <div className="space-y-4">
              {cart.map(item => (
                <div key={item.product.id} className="flex items-center space-x-4 p-4 bg-gray-50 rounded-lg">
                  <img
                    src={item.product.imageUrl}
                    alt={item.product.name}
                    className="w-16 h-16 object-cover rounded"
                  />
                  <div className="flex-1">
                    <h4 className="font-medium">{item.product.name}</h4>
                    <p className="text-sm text-gray-500">₺{item.product.price.toLocaleString('tr-TR')}</p>
                  </div>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={() => updateCartQuantity(item.product.id, -1)}
                      className="p-1 hover:bg-gray-200 rounded"
                    >
                      <Minus className="h-4 w-4" />
                    </button>
                    <span className="w-8 text-center">{item.quantity}</span>
                    <button
                      onClick={() => updateCartQuantity(item.product.id, 1)}
                      className="p-1 hover:bg-gray-200 rounded"
                    >
                      <Plus className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {cart.length > 0 && (
          <div className="border-t p-6">
            <div className="flex justify-between mb-4">
              <span className="text-lg font-semibold">Toplam:</span>
              <span className="text-lg font-bold">₺{getTotalPrice().toLocaleString('tr-TR')}</span>
            </div>
            <button className="w-full bg-blue-600 text-white py-3 rounded-lg hover:bg-blue-700 transition">
              Ödemeye Geç
            </button>
          </div>
        )}
      </div>
    </div>
  )
}