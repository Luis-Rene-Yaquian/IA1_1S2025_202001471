% ======= MediLogic KB (auto-generado) =======
% NO editar a mano; use /admin/kb

sintoma(cefalea).
sintoma(disnea).
sintoma(dolor_garganta).
sintoma(dolor_pecho).
sintoma(dolor_senos).
sintoma(fiebre).
sintoma(nausea).
sintoma(tos).

enfermedad(faringitis, "Faringitis", respiratorio, bacteriano).
enfermedad(gripe, "Gripe", respiratorio, viral).
enfermedad(sinusitis, "Sinusitis", respiratorio, bacteriano).
enf_sintoma(faringitis, dolor_garganta).
enf_sintoma(faringitis, fiebre).
enf_sintoma(gripe, fiebre).
enf_sintoma(gripe, tos).
enf_sintoma(gripe, dolor_garganta).
enf_sintoma(sinusitis, dolor_senos).
enf_sintoma(sinusitis, fiebre).
enf_sintoma(sinusitis, cefalea).
enf_contra_medicamento(faringitis, ibuprofeno).

medicamento(amoxicilina).
medicamento(azitromicina).
medicamento(ibuprofeno).
medicamento(paracetamol).
trata(amoxicilina, faringitis).
trata(azitromicina, sinusitis).
trata(ibuprofeno, gripe).
trata(paracetamol, gripe).
contraindicado(amoxicilina, alergia_penicilina).
contraindicado(amoxicilina, ulcera_gastrica).
contraindicado(azitromicina, prolongacion_qt).
contraindicado(ibuprofeno, ulcera_gastrica).
contraindicado(paracetamol, alergia_paracetamol).
