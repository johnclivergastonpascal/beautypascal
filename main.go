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
	URL             string            `json:"url"`
	Categoria       string            `json:"categoria"`
	Ubicacion       string            `json:"ubicacion"`
	Titulo          string            `json:"titulo"`
	ImagenesGrandes []string          `json:"imagenes"`
	Colores         []Color           `json:"colores"`
	Tamaños         []string          `json:"tamaños"`
	Precios         []Precio          `json:"precios"`
	BloqueLogistico string            `json:"bloque_logistico"`
	Detalles        map[string]string `json:"detalles,omitempty"`
}

func main() {
	start := time.Now()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Ingrese la categoría: ")
	categoria, _ := reader.ReadString('\n')
	categoria = strings.TrimSpace(categoria)

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
			URL:       url,
			Categoria: categoria,
			Ubicacion: ubicacion,
		}

		// ====== Título ======
		if h1, _ := page.Element("div.product-title-container h1"); h1 != nil {
			prod.Titulo = strings.TrimSpace(h1.MustText())
		}

		// ====== Imágenes grandes ======
		mainImgs, _ := page.Elements(`#ProductImageMain img`)
		for _, img := range mainImgs {
			if src, _ := img.Attribute("src"); src != nil {
				imgURL := *src
				if strings.Contains(imgURL, "_960x960q80") { // 🔹 Solo imágenes grandes
					if strings.HasPrefix(imgURL, "//") {
						imgURL = "https:" + imgURL
					}
					prod.ImagenesGrandes = append(prod.ImagenesGrandes, imgURL)
				}
			}
		}

		// ====== Colores ======
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
		sizeSpans, _ := page.Elements("div[data-testid='non-last-sku-item'] span")
		for _, s := range sizeSpans {
			text := strings.TrimSpace(s.MustText())
			if text != "" {
				prod.Tamaños = append(prod.Tamaños, text)
			}
		}

		// ====== NUEVO: Detalles robustos ======
		prod.Detalles = make(map[string]string)

		// Buscamos todos los divs que tengan id-line-clamp-2
		detailDivs, _ := page.Elements("div.id-line-clamp-2")
		for i := 0; i < len(detailDivs)-1; i++ {
			keyDiv := detailDivs[i]
			valDiv := detailDivs[i+1]

			key := strings.TrimSpace(keyDiv.MustText())
			val := strings.TrimSpace(valDiv.MustText())

			if key != "" && val != "" {
				prod.Detalles[key] = val
				i++ // saltamos el valor ya usado
			}
		}

		// ====== Precios ======
		var precios []Precio

		// 🔹 Caso 1: Rango de precios normal
		priceBlocks, _ := page.Elements(`div[data-testid='range-price']`)
		if len(priceBlocks) > 0 {
			for _, pb := range priceBlocks {
				cantidadEl, _ := pb.Element(`div.id-mb-2`)
				valorActualEl, _ := pb.Element(`span.id-text-2xl`)
				valorAnteriorEl, _ := pb.Element(`span[class*='line-through']`) // más robusto

				cantidad := ""
				if cantidadEl != nil {
					cantidad = strings.TrimSpace(cantidadEl.MustText())
				}

				valor := ""
				if valorAnteriorEl != nil {
					valor = strings.TrimSpace(valorAnteriorEl.MustText()) // 🔹 si hay tachado
				} else if valorActualEl != nil {
					valor = strings.TrimSpace(valorActualEl.MustText()) // 🔹 si no hay tachado, usar actual
				}

				if cantidad != "" || valor != "" {
					precios = append(precios, Precio{
						Cantidad: cantidad,
						Valor:    valor,
					})
				}
			}
		} else {
			// 🔹 Caso 2: Precios escalonados (ladder-price)
			ladderBlocks, _ := page.Elements(`div[data-testid='ladder-price'] div.price-item`)
			for _, lb := range ladderBlocks {
				cantidadEl, _ := lb.Element(`div.id-mb-2`)
				valorActualEl, _ := lb.Element(`span.id-text-highlight-dark, span:not(.id-text-highlight-dark):not(.id-line-through)`)
				valorAnteriorEl, _ := lb.Element(`span[class*='line-through']`)

				cantidad := ""
				if cantidadEl != nil {
					cantidad = strings.TrimSpace(cantidadEl.MustText())
				}

				valor := ""
				if valorAnteriorEl != nil {
					valor = strings.TrimSpace(valorAnteriorEl.MustText()) // si hay tachado
				} else if valorActualEl != nil {
					valor = strings.TrimSpace(valorActualEl.MustText()) // si no hay tachado, usar actual
				}

				if cantidad != "" || valor != "" {
					precios = append(precios, Precio{
						Cantidad: cantidad,
						Valor:    valor,
					})
				}
			}
		}

		// Guardar los precios en el producto
		if len(precios) > 0 {
			prod.Precios = precios
		} else {
			prod.Precios = []Precio{{Cantidad: "No disponible", Valor: "N/A"}}
		}

		// ====== Bloque Logístico ======
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
		} else {
			prod.BloqueLogistico = "No disponible"
		}

		productos = append(productos, prod)
		page.Close()
	}

	// ====== Guardar resultados ======
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
