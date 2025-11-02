package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type Precio struct {
	Cantidad string `json:"cantidad"`
	Valor    string `json:"valor"`
}

type Color struct {
	Nombre string `json:"nombre"`
	Imagen string `json:"imagen,omitempty"`
}

type Producto struct {
	URL             string   `json:"url"`
	Categoria       string   `json:"categoria"`
	Subcategoria    string   `json:"subcategoria"`
	Ubicacion       string   `json:"ubicacion"`
	Titulo          string   `json:"titulo"`
	ImagenesGrandes []string `json:"imagenes"`
	Colores         []Color  `json:"colores"`
	Tamaños         []string `json:"tamaños"`
	DescripcionURL  string   `json:"descripcion_url"`
	Precios         []Precio `json:"precios"`
	BloqueLogistico string   `json:"bloque_logistico"`
}

func main() {
	start := time.Now()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Ingrese la categoría: ")
	categoria, _ := reader.ReadString('\n')
	categoria = strings.TrimSpace(categoria)

	fmt.Print("Ingrese la subcategoría: ")
	subcategoria, _ := reader.ReadString('\n')
	subcategoria = strings.TrimSpace(subcategoria)

	fmt.Print("Ubicación: ")
	ubicacion, _ := reader.ReadString('\n')
	ubicacion = strings.TrimSpace(ubicacion)

	// ====== Leer URLs ======
	urlFile := "urls.json"
	data, err := os.ReadFile(urlFile)
	if err != nil {
		log.Fatalf("❌ No se pudo leer %s: %v", urlFile, err)
	}
	var urls []string
	if err := json.Unmarshal(data, &urls); err != nil {
		log.Fatalf("❌ Error al decodificar JSON: %v", err)
	}
	fmt.Printf("🔗 Se encontraron %d URLs en %s\n", len(urls), urlFile)

	// ====== Iniciar navegador Rod ======
	bravePath := `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`
	path := launcher.New().Bin(bravePath).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(path).MustConnect()

	var productos []Producto

	for i, url := range urls {
		fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("🕵️ [%d/%d] Scrapeando producto: %s\n", i+1, len(urls), url)
		page := browser.MustPage(url)
		page.MustWaitLoad()
		time.Sleep(5 * time.Second)

		prod := Producto{
			URL:          url,
			Categoria:    categoria,
			Subcategoria: subcategoria,
			Ubicacion:    ubicacion,
		}

		// ====== Título ======
		fmt.Println("📘 Obteniendo título...")
		if h1, err := page.Element("div.product-title-container h1"); err == nil && h1 != nil {
			prod.Titulo = strings.TrimSpace(h1.MustText())
			fmt.Printf("✅ Título: %s\n", prod.Titulo)
		}

		// ====== Imágenes grandes ======
		fmt.Println("🖼️ Obteniendo imágenes grandes...")
		mainImgs, _ := page.Elements(`#ProductImageMain img`)
		for _, img := range mainImgs {
			if src, _ := img.Attribute("src"); src != nil {
				imgURL := *src
				if strings.HasPrefix(imgURL, "//") {
					imgURL = "https:" + imgURL
				}
				if strings.Contains(imgURL, "960x960") || strings.Contains(imgURL, "kf/") {
					prod.ImagenesGrandes = append(prod.ImagenesGrandes, imgURL)
				}
			}
		}

		// ====== Colores ======
		fmt.Println("🎨 Obteniendo colores...")
		colorImgs, _ := page.Elements("div.double-bordered-box img")
		for _, c := range colorImgs {
			color := Color{}
			if alt, _ := c.Attribute("alt"); alt != nil {
				color.Nombre = *alt
			}
			if src, _ := c.Attribute("src"); src != nil {
				color.Imagen = *src
				if strings.HasPrefix(color.Imagen, "//") {
					color.Imagen = "https:" + color.Imagen
				}
			}
			prod.Colores = append(prod.Colores, color)
		}

		// ====== Tamaños ======
		fmt.Println("📏 Obteniendo tamaños...")
		sizeSpans, _ := page.Elements("div[data-testid='non-last-sku-item'] span")
		for _, s := range sizeSpans {
			text := strings.TrimSpace(s.MustText())
			if text != "" {
				prod.Tamaños = append(prod.Tamaños, text)
			}
		}

		// ====== Descripción ======
		fmt.Println("🧾 Obteniendo URL de descripción...")
		if iframe, err := page.Element("iframe"); err == nil && iframe != nil {
			if src, _ := iframe.Attribute("src"); src != nil {
				if strings.HasPrefix(*src, "//") {
					prod.DescripcionURL = "https:" + *src
				} else {
					prod.DescripcionURL = *src
				}
			}
		}

		// ====== Precios ======
		fmt.Println("💰 Obteniendo precios...")
		var precios []Precio

		// Primero intenta con la nueva estructura (.price-item)
		priceItems, _ := page.Elements("div.price-item")
		if len(priceItems) > 0 {
			for _, item := range priceItems {
				precio := Precio{}

				// Cantidad (ej: "2 - 49 unidades")
				if qtyDiv, _ := item.Element("div.id-mb-2"); qtyDiv != nil {
					precio.Cantidad = strings.TrimSpace(qtyDiv.MustText())
				}

				// Valor (ej: "USD 0.41")
				if valSpan, _ := item.Element("div.id-flex-col span"); valSpan != nil {
					val := strings.TrimSpace(valSpan.MustText())
					val = strings.ReplaceAll(val, "USD", "")
					val = strings.ReplaceAll(val, "US", "")
					precio.Valor = strings.TrimSpace(val)
				}

				if precio.Valor != "" {
					precios = append(precios, precio)
				}
			}
		}

		// Si no se encontraron precios con la nueva estructura, usar los selectores antiguos
		if len(precios) == 0 {
			if priceModule, err := page.Element(`div[data-module-name="module_price"]`); err == nil && priceModule != nil {
				// Rango de precios
				if rangePriceDiv, _ := priceModule.Element("div[data-testid='range-price']"); rangePriceDiv != nil {
					precio := Precio{}
					if cantEl, _ := rangePriceDiv.Element("div"); cantEl != nil {
						precio.Cantidad = strings.TrimSpace(cantEl.MustText())
					}
					if valEl, _ := rangePriceDiv.Element("span"); valEl != nil {
						precio.Valor = strings.TrimSpace(valEl.MustText())
					}
					if precio.Valor != "" {
						precios = append(precios, precio)
					}
				}
				// Precio fijo
				if len(precios) == 0 {
					if fixedPriceDiv, _ := priceModule.Element("div[data-testid='fixed-price']"); fixedPriceDiv != nil {
						precio := Precio{}
						if cantEl, _ := fixedPriceDiv.Element("div"); cantEl != nil {
							precio.Cantidad = strings.TrimSpace(cantEl.MustText())
						}
						if valEl, _ := fixedPriceDiv.Element("strong"); valEl != nil {
							precio.Valor = strings.TrimSpace(valEl.MustText())
						}
						if precio.Valor != "" {
							precios = append(precios, precio)
						}
					}
				}
			}
		}

		if len(precios) > 0 {
			prod.Precios = precios
			fmt.Printf("✅ %d precios encontrados\n", len(precios))
		} else {
			fmt.Println("⚠️ No se encontraron precios.")
		}

		// ====== 🚚 Nuevo Bloque Logístico ======
		fmt.Println("🚚 Obteniendo bloque logístico...")
		logistics, _ := page.Elements(`div.shipping-layout div.shipping-item`)
		var envioTexto []string
		for _, l := range logistics {
			methodEl, _ := l.Element(`.shipping-title_method`)
			introEl, _ := l.Element(`.shipping-intro`)
			deliveryEl, _ := l.Element(`.shipping-delivery`)
			method, intro, delivery := "", "", ""
			if methodEl != nil {
				method = strings.TrimSpace(methodEl.MustText())
			}
			if introEl != nil {
				intro = strings.TrimSpace(introEl.MustText())
			}
			if deliveryEl != nil {
				delivery = strings.TrimSpace(deliveryEl.MustText())
			}
			if method != "" {
				envioTexto = append(envioTexto, fmt.Sprintf("%s: %s | %s", method, intro, delivery))
			}
		}
		if len(envioTexto) > 0 {
			prod.BloqueLogistico = strings.Join(envioTexto, " || ")
			fmt.Printf("✅ Bloque logístico: %s\n", prod.BloqueLogistico)
		} else {
			prod.BloqueLogistico = "No disponible"
			fmt.Println("⚠️ Bloque logístico no encontrado.")
		}

		productos = append(productos, prod)
		page.Close()
	}

	// ====== Guardar resultados ======
	fmt.Println("\n💾 Guardando resultados...")
	file, err := os.Create("productos_detalle.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(productos); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n📁 Se guardaron %d productos en productos_detalle.json\n", len(productos))
	fmt.Println("🕒 Tiempo total:", time.Since(start))

	browser.MustClose()
}
