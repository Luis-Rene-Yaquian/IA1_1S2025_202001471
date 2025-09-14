# Práctica 2 – SSAS y Power BI

## 1. Descripción del Cubo OLAP
El cubo OLAP fue diseñado sobre un modelo estrella compuesto por dos hechos principales:  
- **HechosVentas**: contiene métricas de ventas como unidades, precio unitario y total.  
- **HechosCompras**: contiene métricas de compras como unidades, costo unitario y total.  

Ambos hechos se relacionan con las dimensiones:  
- **DimProducto**: producto, marca, categoría.  
- **DimSucursal**: sucursal, departamento, región.  
- **DimFecha**: jerarquía de tiempo (año, trimestre, mes, día).  
- **DimVendedor / DimCliente / DimProveedor**: entidades de negocio relacionadas.  

Este modelo permite analizar la información de manera flexible según múltiples perspectivas de negocio (tiempo, ubicación, producto, actores de venta/compra).

![alt text](image.png)

![alt text](image-1.png)

---

## 2. Definición de KPIs

### KPI 1 – Costo unitario trimestre
- **Medida base**: Costo 
- **Justificación**: Se mira el aumento de los precios durante un periodo de 3 meses. 

![alt text](image-2.png)


---

## 3. Balanced Scorecard (BSC)

El Balanced Scorecard se estructura en 4 perspectivas:

- **Financiera**:  
  - KPI: Ventas Totales  
  - KPI: Margen Bruto  

- **Clientes**:  
  - KPI: Ticket Promedio  
  - KPI: Ventas por Tipo de Cliente (Minorista vs Mayorista)  

- **Procesos Internos**:  
  - KPI: Unidades Compradas vs Unidades Vendidas  
  - KPI: Costo Unitario Promedio por Categoría  

- **Aprendizaje y Crecimiento**:  
  - KPI: Ventas por Vendedor  
  - KPI: Crecimiento de sucursales por región  


---

## 4. Rolling Forecast

El **Rolling Forecast** es una técnica de planeación financiera y de negocios que busca superar las limitaciones de los presupuestos fijos o estáticos. A diferencia de un presupuesto tradicional que se elabora para un periodo fijo (por ejemplo, un año calendario), el Rolling Forecast se actualiza de manera continua, incorporando nuevos periodos a medida que avanza el tiempo.  

En este caso, se plantea como referencia que el Rolling Forecast se hubiera definido sobre las métricas de ventas y compras trimestrales, proyectando su comportamiento a 4 trimestres futuros. Esto permite que la organización tenga una visión dinámica de su desempeño esperado, en lugar de limitarse a comparaciones con un presupuesto estático.  

### Principios clave del Rolling Forecast
- **Horizonte móvil:** Siempre proyecta hacia adelante un periodo fijo, actualizándose al finalizar cada periodo.  
- **Enfoque en drivers de negocio:** No se centra únicamente en cifras contables, sino en los factores que impulsan las ventas y compras .  
- **Mejor capacidad de adaptación:** Permite identificar de manera temprana cambios en tendencias, riesgos y oportunidades, lo que facilita la toma de decisiones estratégicas.  
- **Complemento a KPIs y BSC:** Se integra con indicadores clave de desempeño y el Balanced Scorecard, ya que no solo mide resultados históricos, sino que proyecta resultados futuros.  


---


